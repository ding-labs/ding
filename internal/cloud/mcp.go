package cloud

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/integrationpolicy"
	"github.com/ding-labs/ding/internal/mcpauth"
	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/ding-labs/ding/internal/mcpserver"
	"github.com/ding-labs/ding/internal/mcpui"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

func (s *Server) MCPHandler(cfg mcpconfig.HTTP, jwks *http.Client) (http.Handler, func(), error) {
	cfg.PublicURL = s.PublicURL
	if cfg.Audience != s.PublicURL+"/mcp" {
		return nil, nil, fmt.Errorf("cloud MCP audience must be its exact public /mcp resource")
	}
	verifier, err := mcpauth.New(cfg, jwks)
	if err != nil {
		return nil, nil, err
	}
	server, err := mcpserver.New(mcpserver.Options{Version: s.Version, HTML: mcpui.WorkspaceHTML, Resolve: s.resolveMCP(cfg.Issuer), ListWorkspaces: s.listMCPWorkspaces(cfg.Issuer)})
	if err != nil {
		verifier.Close()
		return nil, nil, err
	}
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: integrationpolicy.CloudRequestBytes, PropagateRequestCancellation: true, DisableLocalhostProtection: true})
	metadataPath := "/.well-known/oauth-protected-resource/mcp"
	protected := auth.RequireBearerToken(verifier.Verify, &auth.RequireBearerTokenOptions{ResourceMetadataURL: s.PublicURL + metadataPath, Scopes: []string{"ding:inspect"}})(transport)
	metadata := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{Resource: s.PublicURL + "/mcp", AuthorizationServers: []string{cfg.Issuer}, ScopesSupported: []string{"ding:inspect", "ding:preview", "ding:manage", "ding:retry"}, BearerMethodsSupported: []string{"header"}})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == metadataPath {
			metadata.ServeHTTP(w, r)
			return
		}
		if r.URL.Path != "/mcp" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "invalid MCP route or origin", 403)
			return
		}
		protected.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), mcpWorkspaceKey{}, r.Header.Get("X-Ding-Workspace"))))
	}), verifier.Close, nil
}

type mcpWorkspaceKey struct{}

func (s *Server) resolveMCP(issuer string) mcpserver.Resolver {
	return func(ctx context.Context, scope string, token *auth.TokenInfo) (*mcpclient.Client, error) {
		denied := mcpclient.Fail("integration_denied", "Connect this model client in the Ding Cloud Console; inspect its scopes, secret references, and expiry.")
		user, clientID, scopes, err := s.mcpIdentity(ctx, issuer, scope, token)
		if err != nil {
			return nil, err
		}
		selected, _ := ctx.Value(mcpWorkspaceKey{}).(string)
		if requested := mcpserver.RequestedWorkspace(ctx); requested != "" {
			if selected != "" && selected != requested {
				return nil, mcpclient.Fail("invalid_target", "Workspace header and tool argument disagree.")
			}
			selected = requested
		}
		if selected == "" {
			workspaces, err := s.approvedMCPWorkspaces(ctx, user.ID, clientID, scopes)
			if err != nil {
				return nil, err
			}
			if len(workspaces) > 1 {
				return nil, mcpclient.Fail("workspace_required", "Call ding_list_workspaces and provide workspace_id for this operation.")
			}
			if len(workspaces) == 0 {
				return nil, denied
			}
			selected = workspaces[0].ID
		}
		workspace, err := s.DB.ResolveMCPWorkspace(ctx, user.ID, clientID, selected)
		if err != nil {
			return nil, denied
		}
		principal, err := s.DB.Principal(ctx, user.ID, workspace, "integration", scopes)
		if err != nil || !principal.Allows(access.Action(scope)) {
			return nil, denied
		}
		connection, err := s.Vault.MCP(ctx, user.ID, workspace, clientID)
		if err != nil {
			return nil, denied
		}
		tenant, err := s.Pool.Get(ctx, workspace)
		if err != nil {
			return nil, denied
		}
		if _, err := tenant.App.IntegrationAccess(ctx, connection.Token, scope); err != nil {
			return nil, denied
		}
		status, err := tenant.App.Status(ctx)
		if err != nil {
			return nil, mcpclient.Fail("workspace_unavailable", "Workspace identity is unavailable.")
		}
		api := s.api(tenant)
		principal.Grant = connection.GrantID
		return &mcpclient.Client{Target: &mcpcontract.Target{Instance: status.Instance, Workspace: workspace, Execution: s.execution()}, Connection: connection, HTTP: &http.Client{Transport: privateTransport{handler: s.scopedMCP(tenant, api.handler), origin: s.PublicURL, db: s.DB, vault: s.Vault, client: clientID, principal: principal, admit: s.allowWorkspaceRequest}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
	}
}

// Calls stay in-process and keep the existing scoped integration authorization.
// No cloud bearer or tenant credential is sent to a laptop or public network.
type privateTransport struct {
	admit     func(string) bool
	handler   http.Handler
	origin    string
	db        *state.DB
	principal access.Principal
	vault     *state.Vault
	client    string
}

func (t privateTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme+"://"+r.URL.Host != t.origin || !strings.HasPrefix(r.URL.Path, "/v1/integrations/") {
		return nil, fmt.Errorf("invalid internal route")
	}
	t.db.Access.RLock()
	defer t.db.Access.RUnlock()
	p, err := t.db.Principal(r.Context(), t.principal.User, t.principal.Workspace, "integration", t.principal.Scopes)
	if err != nil {
		return nil, fmt.Errorf("membership revoked")
	}
	binding, err := t.vault.MCP(r.Context(), p.User, p.Workspace, t.client)
	if err != nil || binding.GrantID != t.principal.Grant {
		return nil, fmt.Errorf("consent revoked")
	}
	p.Grant = t.principal.Grant
	r = r.Clone(access.WithOperation(access.WithPrincipal(r.Context(), p), r.Method, r.URL.Path))
	w := &privateResponse{header: make(http.Header), status: 200}
	if t.admit != nil && !t.admit(p.Workspace) {
		cloudFail(w, 429, "workspace_rate", "Workspace request rate exceeded; retry later.")
	} else {
		t.handler.ServeHTTP(w, r)
	}
	if w.large {
		return nil, fmt.Errorf("integration response too large")
	}
	return &http.Response{StatusCode: w.status, Header: w.header, Body: io.NopCloser(bytes.NewReader(w.body.Bytes())), Request: r}, nil
}

type privateResponse struct {
	header         http.Header
	status         int
	written, large bool
	body           bytes.Buffer
}

func (w *privateResponse) Header() http.Header { return w.header }
func (w *privateResponse) WriteHeader(code int) {
	if !w.written {
		w.status = code
		w.written = true
	}
}
func (w *privateResponse) Write(b []byte) (int, error) {
	w.WriteHeader(w.status)
	if w.body.Len()+len(b) > mcpclient.MaxResponse {
		w.large = true
		return 0, fmt.Errorf("response limit")
	}
	return w.body.Write(b)
}
