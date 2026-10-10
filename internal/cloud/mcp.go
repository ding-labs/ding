package cloud

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/ding-labs/ding/internal/mcpauth"
	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpserver"
	"github.com/ding-labs/ding/internal/mcpui"
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
	server, err := mcpserver.New(mcpserver.Options{Version: s.Version, HTML: mcpui.WorkspaceHTML, Resolve: s.resolveMCP(cfg.Issuer)})
	if err != nil {
		verifier.Close()
		return nil, nil, err
	}
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 96 << 10, PropagateRequestCancellation: true, DisableLocalhostProtection: true})
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
		protected.ServeHTTP(w, r)
	}), verifier.Close, nil
}

func (s *Server) resolveMCP(issuer string) mcpserver.Resolver {
	return func(ctx context.Context, scope string, token *auth.TokenInfo) (*mcpclient.Client, error) {
		denied := mcpclient.Fail("integration_denied", "Connect this model client in the Ding Cloud Console; inspect its scopes, secret references, and expiry.")
		if token == nil || token.Extra["issuer"] != issuer || !slices.Contains(token.Scopes, "ding:"+scope) {
			return nil, denied
		}
		subject, _ := token.Extra["subject"].(string)
		clientID, _ := token.Extra["client_id"].(string)
		if subject == "" || clientID == "" {
			return nil, denied
		}
		account, err := s.DB.IdentityAccount(ctx, issuer, subject)
		if err != nil {
			return nil, denied
		}
		connection, err := s.Vault.MCP(ctx, account.ID, clientID)
		if err != nil {
			return nil, denied
		}
		tenant, err := s.Pool.Get(ctx, account.ID)
		if err != nil {
			return nil, denied
		}
		if _, err := tenant.App.IntegrationAccess(ctx, connection.Token, scope); err != nil {
			return nil, denied
		}
		api := s.api(tenant)
		return &mcpclient.Client{Connection: connection, HTTP: &http.Client{Transport: privateTransport{api.handler, s.PublicURL}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
	}
}

// Calls stay in-process and keep the existing scoped integration authorization.
// No cloud bearer or tenant credential is sent to a laptop or public network.
type privateTransport struct {
	handler http.Handler
	origin  string
}

func (t privateTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme+"://"+r.URL.Host != t.origin || !strings.HasPrefix(r.URL.Path, "/v1/integrations/") {
		return nil, fmt.Errorf("invalid internal route")
	}
	w := &privateResponse{header: make(http.Header), status: 200}
	t.handler.ServeHTTP(w, r)
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
