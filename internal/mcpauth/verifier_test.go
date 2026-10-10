package mcpauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

func TestAsymmetricAlgorithmsAndTampering(t *testing.T) {
	rsaKey := keyForTest(t, "rsa")
	ec256, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ec384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	_, ed, _ := ed25519.GenerateKey(rand.Reader)
	for _, tc := range []struct {
		name string
		key  any
	}{
		{"RS256", rsaKey}, {"RS384", rsaKey}, {"RS512", rsaKey}, {"ES256", ec256}, {"ES384", ec384}, {"EdDSA", ed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := tc.key.(jwk.Key)
			if !ok {
				var err error
				key, err = jwk.Import(tc.key)
				if err != nil {
					t.Fatal(err)
				}
			}
			_ = key.Set(jwk.KeyIDKey, "test")
			public, err := jwk.PublicKeyOf(key)
			if err != nil {
				t.Fatal(err)
			}
			set := jwk.NewSet()
			_ = set.AddKey(public)
			issuer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(set) }))
			defer issuer.Close()
			cfg := mcpconfig.HTTP{PublicURL: "https://ding.example", Issuer: issuer.URL, JWKSURI: issuer.URL, Audience: "ding", Algorithm: tc.name, Subjects: map[string]string{"alice": filepath.Join(t.TempDir(), "config")}}
			v, err := New(cfg, issuer.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			token := jwt.New()
			for k, value := range map[string]any{"iss": cfg.Issuer, "aud": cfg.Audience, "sub": "alice", "scope": "ding:inspect", "exp": time.Now().Add(time.Hour)} {
				_ = token.Set(k, value)
			}
			alg, _ := jwa.LookupSignatureAlgorithm(tc.name)
			raw, err := jwt.Sign(token, jwt.WithKey(alg, key))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := v.Verify(context.Background(), string(raw), nil); err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(string(raw), ".")
			sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
			sig[0] ^= 1
			if _, err := v.Verify(context.Background(), parts[0]+"."+parts[1]+"."+base64.RawURLEncoding.EncodeToString(sig), nil); err == nil {
				t.Fatal("bad signature accepted")
			}
			for _, badAlg := range []string{"none", "HS256", "unknown"} {
				h, _ := json.Marshal(map[string]string{"alg": badAlg, "kid": "test", "jku": "https://attacker.example/jwks"})
				if _, err := v.Verify(context.Background(), base64.RawURLEncoding.EncodeToString(h)+"."+parts[1]+"."+parts[2], nil); err == nil {
					t.Fatal("bad algorithm accepted")
				}
			}
		})
	}
}

func keyForTest(t *testing.T, id string) jwk.Key {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	j, err := jwk.Import(key)
	if err != nil {
		t.Fatal(err)
	}
	_ = j.Set(jwk.KeyIDKey, id)
	return j
}
func tokenForTest(t *testing.T, key jwk.Key, issuer, audience string, changes map[string]any) string {
	t.Helper()
	token := jwt.New()
	claims := map[string]any{"iss": issuer, "aud": audience, "sub": "alice", "scope": "ding:inspect", "exp": time.Now().Add(time.Hour)}
	for k, v := range changes {
		claims[k] = v
	}
	for k, v := range claims {
		if v != nil {
			if err := token.Set(k, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	raw, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), key))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestJWTClaimsRotationAndBoundedRefresh(t *testing.T) {
	key := keyForTest(t, "first")
	next := keyForTest(t, "next")
	var calls atomic.Int32
	var mu sync.Mutex
	current := key
	unavailable := false
	issuer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		mu.Lock()
		defer mu.Unlock()
		if unavailable {
			http.Error(w, "private upstream diagnostics", 503)
			return
		}
		public, err := jwk.PublicKeyOf(current)
		if err != nil {
			t.Error(err)
			return
		}
		set := jwk.NewSet()
		_ = set.AddKey(public)
		_ = json.NewEncoder(w).Encode(set)
	}))
	defer issuer.Close()
	cfg := mcpconfig.HTTP{PublicURL: "https://ding.example", Issuer: issuer.URL, JWKSURI: issuer.URL + "/jwks", Audience: "https://ding.example/mcp", Algorithm: "RS256", Subjects: map[string]string{"alice": filepath.Join(t.TempDir(), "config")}}
	v, err := New(cfg, issuer.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	clock := time.Now()
	v.now = func() time.Time { return clock }
	verify := func(raw string) error { _, err := v.Verify(context.Background(), raw, nil); return err }
	if err := verify(tokenForTest(t, key, cfg.Issuer, cfg.Audience, nil)); err != nil {
		t.Fatal(err)
	}
	for _, changes := range []map[string]any{{"iss": "https://wrong.example"}, {"aud": "wrong"}, {"sub": ""}, {"exp": time.Now().Add(-time.Hour)}, {"exp": nil}, {"nbf": time.Now().Add(time.Hour)}} {
		if err := verify(tokenForTest(t, key, cfg.Issuer, cfg.Audience, changes)); err == nil {
			t.Fatal("invalid claims accepted", changes)
		}
	}
	unknown := tokenForTest(t, next, cfg.Issuer, cfg.Audience, nil)
	for range 10 {
		if verify(unknown) == nil {
			t.Fatal("unknown key accepted")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("unknown key caused unbounded refresh", calls.Load())
	}
	mu.Lock()
	current = next
	mu.Unlock()
	clock = clock.Add(refreshInterval + time.Second)
	if err := verify(unknown); err != nil {
		t.Fatal("rotation", err)
	}
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
	if verify(tokenForTest(t, key, cfg.Issuer, cfg.Audience, nil)) == nil {
		t.Fatal("removed key accepted")
	}
	mu.Lock()
	unavailable = true
	mu.Unlock()
	clock = clock.Add(cacheTTL + time.Second)
	if err := verify(unknown); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("stale cache or error leak", err)
	}
	if calls.Load() != 3 {
		t.Fatal(calls.Load())
	}
}

func TestJWKSRedirectNeverFollowed(t *testing.T) {
	var leaked atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked.Add(1) }))
	defer target.Close()
	issuer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer issuer.Close()
	cfg := mcpconfig.HTTP{PublicURL: "https://ding.example", Issuer: issuer.URL, JWKSURI: issuer.URL, Audience: "ding", Subjects: map[string]string{"alice": filepath.Join(t.TempDir(), "config")}}
	v, err := New(cfg, issuer.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if _, err := v.Verify(context.Background(), tokenForTest(t, keyForTest(t, "a"), cfg.Issuer, cfg.Audience, nil), nil); err == nil {
		t.Fatal("redirect accepted")
	}
	if leaked.Load() != 0 {
		t.Fatal("redirect followed")
	}
}
