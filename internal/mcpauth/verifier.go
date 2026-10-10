// Package mcpauth verifies bearer tokens from an operator-selected identity
// provider. OAuth login and token issuance remain with that provider.
package mcpauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

const cacheTTL = 5 * time.Minute
const refreshInterval = 30 * time.Second

type Verifier struct {
	config            mcpconfig.HTTP
	http              *http.Client
	mu                sync.Mutex
	keys              jwk.Set
	loaded, attempted time.Time
	now               func() time.Time
}

func New(config mcpconfig.HTTP, client *http.Client) (*Verifier, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if client == nil {
		client = mcpclient.HTTPClient()
	}
	copyClient := *client
	client = &copyClient
	client.Timeout = 5 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Verifier{config: config, http: client, now: time.Now}, nil
}
func (v *Verifier) Close() { v.http.CloseIdleConnections() }

// Verify returns only constant errors: auth middleware may put them in HTTP
// responses. The unverified header selects a key, never a URL or algorithm policy.
func (v *Verifier) Verify(ctx context.Context, raw string, _ *http.Request) (*auth.TokenInfo, error) {
	invalid := auth.ErrInvalidToken
	if len(raw) > 16<<10 {
		return nil, invalid
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, invalid
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, invalid
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != v.config.Algorithm || len(header.KeyID) > 256 {
		return nil, invalid
	}
	algorithm, ok := jwa.LookupSignatureAlgorithm(v.config.Algorithm)
	if !ok {
		return nil, invalid
	}
	key := v.key(ctx, header.KeyID)
	if key == nil {
		return nil, invalid
	}
	if alg, present := key.Algorithm(); present && alg.String() != header.Algorithm {
		return nil, invalid
	}
	if usage, present := key.KeyUsage(); present && usage != "sig" {
		return nil, invalid
	}
	if ops, present := key.KeyOps(); present && !slices.Contains(ops, jwk.KeyOpVerify) {
		return nil, invalid
	}
	token, err := jwt.ParseString(raw, jwt.WithKey(algorithm, key), jwt.WithValidate(true), jwt.WithIssuer(v.config.Issuer), jwt.WithAudience(v.config.Audience), jwt.WithRequiredClaim(jwt.ExpirationKey), jwt.WithRequiredClaim(jwt.SubjectKey))
	if err != nil {
		return nil, invalid
	}
	subject, _ := token.Subject()
	expiry, _ := token.Expiration()
	if subject == "" || !expiry.After(v.now()) {
		return nil, invalid
	}
	var scope string
	if token.Get("scope", &scope) != nil {
		return nil, invalid
	}
	return &auth.TokenInfo{Scopes: strings.Fields(scope), Expiration: expiry, UserID: v.config.Issuer + "\x00" + subject, Extra: map[string]any{"subject": subject, "issuer": v.config.Issuer}}, nil
}

// Fetches are bounded, serialized, and rate-limited even for unknown key IDs.
// A stale cache is never used after a failed refresh. New keys may take up to
// 30 seconds to appear; known keys expire from this cache within five minutes.
func (v *Verifier) key(ctx context.Context, id string) jwk.Key {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	lookup := func() jwk.Key {
		if v.keys == nil {
			return nil
		}
		if id == "" {
			if v.keys.Len() == 1 {
				k, _ := v.keys.Key(0)
				return k
			}
			return nil
		}
		k, _ := v.keys.LookupKeyID(id)
		return k
	}
	if now.Sub(v.loaded) < cacheTTL {
		if k := lookup(); k != nil {
			return k
		}
	}
	if !v.attempted.IsZero() && now.Sub(v.attempted) < refreshInterval {
		return nil
	}
	v.attempted = now
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.config.JWKSURI, nil)
	if err != nil {
		return nil
	}
	response, err := v.http.Do(req)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil
	}
	keys, err := jwk.Parse(raw, jwk.WithRejectDuplicateKID(true))
	if err != nil || keys.Len() == 0 || keys.Len() > 100 {
		return nil
	}
	v.keys = keys
	v.loaded = now
	return lookup()
}
