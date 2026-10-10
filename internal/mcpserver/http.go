package mcpserver

import (
	"context"
	"maps"
	"net/http"
	"net/url"
	"slices"

	"github.com/ding-labs/ding/internal/mcpauth"
	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

type HTTPOptions struct {
	Config        mcpconfig.HTTP
	Version, HTML string
	// Optional transports support controlled TLS issuers and boundary tests.
	JWKSClient, DaemonClient *http.Client
}

func HTTPHandler(o HTTPOptions) (http.Handler, func(), error) {
	cfg := o.Config
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	cfg.Subjects = maps.Clone(cfg.Subjects)
	for _, path := range cfg.Subjects {
		if _, err := mcpconfig.LoadConnection(path); err != nil {
			return nil, nil, mcpconfig.ErrConfig
		}
	}
	verifier, err := mcpauth.New(cfg, o.JWKSClient)
	if err != nil {
		return nil, nil, err
	}
	client := o.DaemonClient
	if client == nil {
		client = mcpclient.HTTPClient()
	}
	close := func() { verifier.Close(); client.CloseIdleConnections() }
	s, err := New(Options{Version: o.Version, HTML: o.HTML, Resolve: func(ctx context.Context, scope string, token *auth.TokenInfo) (*mcpclient.Client, error) {
		if token == nil || !slices.Contains(token.Scopes, "ding:"+scope) || token.Extra["issuer"] != cfg.Issuer {
			return nil, mcpclient.Fail("integration_denied", "this identity has no paired Ding grant for this action")
		}
		subject, _ := token.Extra["subject"].(string)
		path, ok := cfg.Subjects[subject]
		if !ok {
			return nil, mcpclient.Fail("integration_denied", "this identity has no paired Ding grant for this action")
		}
		connection, err := mcpconfig.LoadConnection(path)
		if err != nil {
			return nil, mcpclient.Fail("connection_unavailable", "the operator must repair the identity binding")
		}
		return &mcpclient.Client{Connection: connection, HTTP: client}, nil
	}})
	if err != nil {
		close()
		return nil, nil, err
	}
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 12 << 20, PropagateRequestCancellation: true, DisableLocalhostProtection: true})
	metadataPath := "/.well-known/oauth-protected-resource/mcp"
	protected := auth.RequireBearerToken(verifier.Verify, &auth.RequireBearerTokenOptions{ResourceMetadataURL: cfg.PublicURL + metadataPath, Scopes: []string{"ding:inspect"}})(transport)
	metadata := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{Resource: cfg.PublicURL + "/mcp", AuthorizationServers: []string{cfg.Issuer}, ScopesSupported: []string{"ding:inspect", "ding:preview", "ding:manage", "ding:retry"}, BearerMethodsSupported: []string{"header"}})
	u, _ := url.Parse(cfg.PublicURL)
	// The proxy preserves this exact public Host. This outer guard replaces the
	// SDK's localhost-only check; forwarded headers never grant trust.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if r.Host != u.Host {
			http.Error(w, "unrecognized host", 403)
			return
		}
		if r.URL.Path == metadataPath {
			metadata.ServeHTTP(w, r)
			return
		}
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != cfg.PublicURL {
			http.Error(w, "unrecognized origin", 403)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "cross-origin request denied", 403)
			return
		}
		protected.ServeHTTP(w, r)
	})
	return handler, close, nil
}
