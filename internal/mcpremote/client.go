package mcpremote

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

// WithSession serializes profile refresh across processes, reloads rotated
// credentials, and never opens an interactive login from a model request.
func WithSession(ctx context.Context, path string, fn func(*mcp.ClientSession, Profile) error) error {
	base := mcpclient.HTTPClient()
	defer base.CloseIdleConnections()
	return withSession(ctx, path, base, fn)
}

func withSession(ctx context.Context, path string, base *http.Client, fn func(*mcp.ClientSession, Profile) error) error {
	unlock, err := mcpconfig.Lock(path + ".lock")
	if err != nil {
		return mcpclient.Fail("connection_busy", "another cloud request is active; retry this same request")
	}
	defer unlock()
	p, err := Load(path)
	if err != nil {
		return mcpclient.Fail("cloud_signin_required", "connect this cloud profile using ding mcp cloud connect")
	}
	config := oauth2.Config{ClientID: p.ClientID, Endpoint: oauth2.Endpoint{TokenURL: p.TokenURL, AuthStyle: p.AuthStyle}, Scopes: p.Scopes}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, base)
	source := &savingSource{source: config.TokenSource(ctx, p.Token), profile: p, path: path}
	client := &http.Client{Timeout: base.Timeout, CheckRedirect: base.CheckRedirect, Transport: &resourceTransport{endpoint: p.URL, base: &oauth2.Transport{Source: source, Base: base.Transport}}}
	cs, err := dial(ctx, p.URL, client, nil)
	if err != nil {
		return mcpclient.Fail("cloud_unavailable", "cloud connection failed; check its availability or reconnect this OAuth profile")
	}
	defer cs.Close()
	return fn(cs, p)
}

type resourceTransport struct {
	endpoint string
	base     http.RoundTripper
}

func (t *resourceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.String() != t.endpoint {
		return nil, mcpconfig.ErrConfig
	}
	return t.base.RoundTrip(r)
}

type savingSource struct {
	mu      sync.Mutex
	source  oauth2.TokenSource
	profile Profile
	path    string
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err := s.source.Token()
	if err != nil {
		return nil, err
	}
	old := s.profile.Token
	if old.AccessToken != token.AccessToken || old.RefreshToken != token.RefreshToken || !old.Expiry.Equal(token.Expiry) {
		next := s.profile
		next.Token = token
		if err := next.Validate(); err != nil {
			return nil, err
		}
		if err := mcpconfig.AtomicJSON(s.path, next); err != nil {
			return nil, err
		}
		s.profile = next
	}
	return token, nil
}

func dial(ctx context.Context, endpoint string, client *http.Client, handler auth.OAuthHandler) (*mcp.ClientSession, error) {
	return mcp.NewClient(&mcp.Implementation{Name: "Ding cloud bridge", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: client, OAuthHandler: handler, MaxRetries: -1, DisableStandaloneSSE: true, MaxEventSize: mcpclient.MaxResponse}, nil)
}

// View accepts only the bounded Ding contract; remote errors remain text.
func View(result *mcp.CallToolResult) (mcpcontract.View, error) {
	var view mcpcontract.View
	if result == nil {
		return view, mcpclient.Fail("invalid_response", "cloud returned no tool result")
	}
	if result.IsError {
		for _, content := range result.Content {
			if text, ok := content.(*mcp.TextContent); ok && len(text.Text) <= 2000 {
				return view, mcpclient.Fail("remote_error", text.Text)
			}
		}
		return view, mcpclient.Fail("remote_error", "cloud tool failed")
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil || len(raw) > mcpclient.MaxResponse || json.Unmarshal(raw, &view) != nil || view.View == "" || view.Data == nil {
		return view, mcpclient.Fail("invalid_response", "cloud returned an incompatible Ding result")
	}
	return view, nil
}

func Capabilities(ctx context.Context, cs *mcp.ClientSession, p Profile) (mcpcontract.View, error) {
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ding_get_capabilities", Arguments: map[string]any{"workspace_id": p.Workspace}})
	if err != nil {
		return mcpcontract.View{}, err
	}
	view, err := View(result)
	if err != nil {
		return view, err
	}
	if view.Target == nil || view.Target.Workspace != p.Workspace || (p.Instance != "" && view.Target.Instance != p.Instance) || !strings.Contains(" cloud shared ", " "+view.Target.Execution+" ") {
		return view, mcpclient.Fail("target_changed", "the configured cloud workspace identity changed; reconnect after reviewing the target")
	}
	return view, nil
}
