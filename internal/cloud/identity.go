package cloud

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"golang.org/x/oauth2"
)

type Identity struct{ Issuer, Subject string }
type IdentityProvider interface {
	Begin(state, nonce, verifier string) string
	Exchange(context.Context, string, string, string) (Identity, error)
}
type IdentityConfig struct {
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	// Optional IdP-selection scopes select the configured minimal GitHub broker.
	Scopes []string `json:"scopes"`
}

func (c IdentityConfig) Validate() error {
	u, err := url.Parse(c.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(c.ClientID) == "" {
		return fmt.Errorf("configure an HTTPS OIDC issuer and registered client")
	}
	for _, scope := range c.Scopes {
		const prefix = "urn:zitadel:iam:org:idp:id:"
		if !strings.HasPrefix(scope, prefix) || len(scope) == len(prefix) || strings.ContainsAny(scope, " \t\r\n") {
			return fmt.Errorf("only a GitHub identity-provider selection scope may be added to browser login")
		}
	}
	return nil
}

type identityProvider struct {
	issuer   string
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	client   *http.Client
}

func NewIdentity(ctx context.Context, publicURL string, c IdentityConfig) (IdentityProvider, error) {
	return newIdentity(ctx, publicURL, c, mcpclient.HTTPClient())
}

func newIdentity(ctx context.Context, publicURL string, c IdentityConfig, client *http.Client) (IdentityProvider, error) {
	publicURL, err := mcpconfig.Endpoint(publicURL, true)
	if err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	bounded := *client
	bounded.Timeout = 10 * time.Second
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	next := client.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	bounded.Transport = identityTransport{next}
	client = &bounded
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), c.Issuer)
	if err != nil {
		return nil, fmt.Errorf("identity discovery failed")
	}
	endpoint := provider.Endpoint()
	var discovery struct {
		JWKS string `json:"jwks_uri"`
	}
	if err := provider.Claims(&discovery); err != nil {
		return nil, fmt.Errorf("invalid identity discovery")
	}
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL, discovery.JWKS} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return nil, fmt.Errorf("identity endpoints require HTTPS")
		}
	}
	scopes := []string{oidc.ScopeOpenID}
	for _, scope := range c.Scopes {
		// Scope configuration cannot accidentally request GitHub repositories or
		// private email. The broker's GitHub upstream settings remain a launch gate.
		scopes = append(scopes, scope)
	}
	return &identityProvider{issuer: c.Issuer, oauth: oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: endpoint, RedirectURL: publicURL + "/auth/callback", Scopes: scopes}, verifier: provider.Verifier(&oidc.Config{ClientID: c.ClientID}), client: client}, nil
}

type identityTransport struct{ next http.RoundTripper }

func (t identityTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.User != nil {
		return nil, fmt.Errorf("identity requests require HTTPS")
	}
	response, err := t.next.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	if response.ContentLength > 1<<20 {
		response.Body.Close()
		return nil, fmt.Errorf("identity response exceeds limit")
	}
	response.Body = &identityBody{ReadCloser: response.Body, remaining: 1 << 20}
	return response, nil
}

type identityBody struct {
	io.ReadCloser
	remaining int64
}

func (b *identityBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, fmt.Errorf("identity response exceeds limit")
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return n, fmt.Errorf("identity response exceeds limit")
	}
	return n, err
}

func (p *identityProvider) Begin(state, nonce, verifier string) string {
	return p.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}
func (p *identityProvider) Exchange(ctx context.Context, code, nonce, verifier string) (Identity, error) {
	ctx = oidc.ClientContext(ctx, p.client)
	token, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("sign-in exchange failed; start a new sign-in")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || len(raw) > 32<<10 {
		return Identity{}, fmt.Errorf("identity token missing or oversized")
	}
	identity, err := p.verifier.Verify(ctx, raw)
	if err != nil || identity.Subject == "" || len(identity.Subject) > 512 || subtle.ConstantTimeCompare([]byte(identity.Nonce), []byte(nonce)) != 1 {
		return Identity{}, fmt.Errorf("identity verification failed; start a new sign-in")
	}
	return Identity{p.issuer, identity.Subject}, nil
}
