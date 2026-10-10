package cloud

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
type identityProvider struct {
	issuer   string
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	client   *http.Client
}

func NewIdentity(ctx context.Context, publicURL string, c IdentityConfig) (IdentityProvider, error) {
	publicURL, err := mcpconfig.Endpoint(publicURL, true)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(c.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || c.ClientID == "" {
		return nil, fmt.Errorf("configure an HTTPS OIDC issuer and registered client")
	}
	client := mcpclient.HTTPClient()
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), c.Issuer)
	if err != nil {
		return nil, fmt.Errorf("identity discovery failed")
	}
	endpoint := provider.Endpoint()
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return nil, fmt.Errorf("identity endpoints require HTTPS")
		}
	}
	scopes := []string{oidc.ScopeOpenID}
	for _, scope := range c.Scopes {
		// Scope configuration cannot accidentally request GitHub repositories or
		// private email. The broker's GitHub upstream settings remain a launch gate.
		if !strings.HasPrefix(scope, "urn:zitadel:iam:org:idp:id:") {
			return nil, fmt.Errorf("only a GitHub identity-provider selection scope may be added to browser login")
		}
		scopes = append(scopes, scope)
	}
	return &identityProvider{issuer: c.Issuer, oauth: oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: endpoint, RedirectURL: publicURL + "/auth/callback", Scopes: scopes}, verifier: provider.Verifier(&oidc.Config{ClientID: c.ClientID}), client: client}, nil
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
