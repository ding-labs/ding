package cloud

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

func TestOIDCVerifiesSignedIdentityPKCEAndNonce(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	public, err := jwk.Import(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	_ = public.Set(jwk.KeyIDKey, "test")
	set := jwk.NewSet()
	_ = set.AddKey(public)
	var issuer, challenge, nonce string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(set)
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			token := jwt.New()
			_ = token.Set(jwt.IssuerKey, issuer)
			_ = token.Set(jwt.SubjectKey, "stable-github-backed-id")
			_ = token.Set(jwt.AudienceKey, []string{"ding-console"})
			_ = token.Set(jwt.ExpirationKey, time.Now().Add(time.Minute))
			_ = token.Set(jwt.IssuedAtKey, time.Now())
			_ = token.Set("nonce", nonce)
			signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), key))
			if err != nil {
				t.Error(err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "synthetic-provider-token", "token_type": "Bearer", "id_token": string(signed)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	p, err := newIdentity(context.Background(), "https://ding.example", IdentityConfig{Issuer: issuer, ClientID: "ding-console", ClientSecret: "synthetic"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	verifier := "a-valid-verifier-with-at-least-forty-three-characters"
	u, _ := url.Parse(p.Begin("state", "nonce", verifier))
	challenge = u.Query().Get("code_challenge")
	nonce = u.Query().Get("nonce")
	if u.Query().Get("scope") != "openid" || u.Query().Get("code_challenge_method") != "S256" {
		t.Fatal("unexpected identity privileges or missing PKCE", u.RawQuery)
	}
	identity, err := p.Exchange(context.Background(), "good", "nonce", verifier)
	if err != nil || identity.Subject != "stable-github-backed-id" {
		t.Fatal("identity exchange failed", err)
	}
	if _, err := p.Exchange(context.Background(), "good", "wrong-nonce", verifier); err == nil {
		t.Fatal("wrong nonce accepted")
	}
	if _, err := p.Exchange(context.Background(), "good", "nonce", "wrong-verifier"); err == nil {
		t.Fatal("wrong PKCE accepted")
	}
}
