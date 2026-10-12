package mcpremote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// Login uses the SDK's discovery, issuer binding, state and S256 PKCE handling.
// Existing profiles may only be refreshed against exactly the same identity.
// Callback success alone never saves a profile: workspace consent and the
// instance identity must also be checked through the authenticated MCP service.
func Login(ctx context.Context, path string, p Profile, port int, out io.Writer) error {
	if path == "" || port < 1 || port > 65535 {
		return mcpconfig.ErrConfig
	}
	if err := p.ValidateSetup(); err != nil {
		return err
	}
	unlock, err := mcpconfig.Lock(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	old, err := Load(path)
	if err == nil {
		if old.URL != p.URL || old.Issuer != p.Issuer || old.ClientID != p.ClientID || old.Workspace != p.Workspace {
			return mcpclient.Fail("target_changed", "use a new profile for a different server, account client or workspace")
		}
		p.Instance = old.Instance
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer listener.Close()
	callback := &loopbackCallback{result: make(chan *auth.AuthorizationResult, 1)}
	server := &http.Server{Handler: callback, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MaxHeaderBytes: 8 << 10}
	go server.Serve(listener)
	defer server.Close()
	redirect := "http://" + listener.Addr().String() + "/callback"
	client := mcpclient.HTTPClient()
	defer client.CloseIdleConnections()
	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		PreregisteredClient: &oauthex.ClientCredentials{ClientID: p.ClientID, Issuer: p.Issuer},
		RedirectURL:         redirect, Client: client, RequestRefreshToken: true,
		ScopeFilter: func([]string) []string { return append([]string(nil), p.Scopes...) },
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			u, err := url.Parse(args.URL)
			if err != nil || u.Scheme != "https" {
				return nil, mcpconfig.ErrConfig
			}
			callback.mu.Lock()
			callback.state = u.Query().Get("state")
			callback.mu.Unlock()
			if _, err = fmt.Fprintln(out, "Open this sign-in link in your browser:\n"+args.URL); err != nil {
				return nil, err
			}
			select {
			case result := <-callback.result:
				return result, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		NewTokenSource: func(ctx context.Context, config *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			p.TokenURL = config.Endpoint.TokenURL
			p.AuthStyle = config.Endpoint.AuthStyle
			p.Token = token
			return config.TokenSource(ctx, token), nil
		},
	})
	if err != nil {
		return err
	}
	cs, err := dial(ctx, p.URL, client, handler)
	if err != nil {
		return mcpclient.Fail("cloud_signin_failed", "sign-in failed; check OAuth registration, callback address and cloud availability")
	}
	defer cs.Close()
	view, err := Capabilities(ctx, cs, p)
	if err != nil {
		return err
	}
	p.Instance = view.Target.Instance
	source, err := handler.TokenSource(ctx)
	if err != nil {
		return err
	}
	p.Token, err = source.Token()
	if err != nil {
		return err
	}
	if err = p.Validate(); err != nil {
		return err
	}
	if err = mcpconfig.AtomicJSON(path, p); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "Cloud profile saved for the approved workspace. Local pairing is unchanged.")
	return err
}

type loopbackCallback struct {
	mu     sync.Mutex
	state  string
	result chan *auth.AuthorizationResult
}

func (c *loopbackCallback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	if r.Method != "GET" || r.URL.Path != "/callback" || len(r.URL.RawQuery) > 8192 {
		http.Error(w, "Invalid callback", 400)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	q := r.URL.Query()
	if c.state == "" || q.Get("state") != c.state || q.Get("code") == "" || q.Get("error") != "" {
		http.Error(w, "Sign-in was not completed. Return to Ding and retry.", 400)
		return
	}
	select {
	case c.result <- &auth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}:
		c.state = ""
		fmt.Fprintln(w, "Return to Ding to check workspace access and complete the connection.")
	default:
		http.Error(w, "Callback already received", 409)
	}
}
