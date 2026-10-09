package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestCredentialsAndBinding(t *testing.T) {
	dir := t.TempDir()
	c, err := PrivateCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := PrivateCredentials(dir)
	if err != nil || c != again || c.Admin == c.Ingest {
		t.Fatal(again, err)
	}
	if err := SaveConnection(dir, "http://127.0.0.1:1234"); err != nil {
		t.Fatal(err)
	}
	client, err := Connect(dir)
	if err != nil || client.Token != c.Admin || client.URL != "http://127.0.0.1:1234" {
		t.Fatal(client.URL, err)
	}
	if _, err := Connect(t.TempDir()); err == nil {
		t.Fatal("connected to absent daemon")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(dir, "tokens.json"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := PrivateCredentials(dir); err == nil {
			t.Fatal("public token file accepted")
		}
		os.Chmod(filepath.Join(dir, "tokens.json"), 0600)
	}
	os.WriteFile(filepath.Join(dir, "tokens.json"), []byte("bad"), 0600)
	if _, err := PrivateCredentials(dir); err == nil {
		t.Fatal("replaced broken tokens")
	}
	if _, err := Connect(dir); err == nil {
		t.Fatal("connected with broken tokens")
	}
	for _, tc := range []struct {
		addr          string
		remote, valid bool
	}{{"127.0.0.1:0", false, true}, {"[::1]:7676", false, true}, {"0.0.0.0:7676", false, false}, {"0.0.0.0:7676", true, true}, {"localhost:7676", true, false}, {":7676", true, false}, {"bad", true, false}} {
		if err := ValidateListen(tc.addr, tc.remote); (err == nil) != tc.valid {
			t.Fatal(tc, err)
		}
	}
}

const example = `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: api}
spec:
  source: {type: http, url: https://example.com}
  condition: {field: http.status, operator: gte, value: 500}
`

func TestAPIAuthApplyInspectAndErrors(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := Credentials{Admin: strings.Repeat("a", 64), Ingest: strings.Repeat("i", 64)}
	server := httptest.NewServer(Handler(watchrun.New(s), c))
	defer server.Close()
	client := Client{URL: server.URL, Token: c.Admin, HTTP: server.Client()}
	for _, token := range []string{"", c.Ingest, "bad"} {
		unauth := client
		unauth.Token = token
		if _, err := unauth.Call(ctx, "GET", "/v1/watches", nil); err == nil {
			t.Fatal("unauthorized accepted")
		}
	}
	if _, err := client.Call(ctx, "GET", "/health", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(ctx, "POST", "/v1/apply", watchrun.ApplyRequest{Manifest: example}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/watches", "/v1/watches/api", "/v1/events?watch=api"} {
		data, err := client.Call(ctx, "GET", path, nil)
		if err != nil || !json.Valid(data) {
			t.Fatal(path, string(data), err)
		}
	}
	for _, tc := range []struct {
		method, path, code string
		data               any
	}{{"GET", "/v1/watches/missing", "not_found", nil}, {"GET", "/v1/events?after=bad", "invalid_cursor", nil}, {"POST", "/v1/apply", "invalid_request", map[string]any{"unexpected": true}}, {"POST", "/v1/apply", "apply_failed", watchrun.ApplyRequest{Manifest: "bad"}}, {"POST", "/v1/apply", "revision_conflict", watchrun.ApplyRequest{Manifest: example, Expected: map[string]string{"api": "bad"}}}} {
		_, err := client.Call(ctx, tc.method, tc.path, tc.data)
		var api *APIError
		if !errors.As(err, &api) || api.Code != tc.code {
			t.Fatal(tc, err)
		}
	}
	for _, action := range []string{"pause", "resume", "delete"} {
		if _, err := client.Call(ctx, "POST", "/v1/watches/api/lifecycle", watchrun.LifecycleRequest{Action: action}); err != nil {
			t.Fatal(action, err)
		}
	}
	for _, request := range []watchrun.LifecycleRequest{{Action: "resume"}, {Action: "delete", Expected: "stale"}, {Action: "wat"}} {
		if _, err := client.Call(ctx, "POST", "/v1/watches/api/lifecycle", request); err == nil {
			t.Fatal("invalid lifecycle accepted")
		}
	}
	if _, err := client.Call(ctx, "POST", "/v1/watches/absent/lifecycle", watchrun.LifecycleRequest{Action: "pause"}); err == nil {
		t.Fatal("missing watch accepted")
	}
	// Multiple JSON objects are not accepted as one request.
	req, _ := http.NewRequest("POST", server.URL+"/v1/apply", bytes.NewBufferString(`{} {}`))
	req.Header.Set("Authorization", "Bearer "+c.Admin)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal(response.Status)
	}
	s.Close()
	for _, path := range []string{"/v1/watches", "/v1/watches/api", "/v1/events"} {
		_, err := client.Call(ctx, "GET", path, nil)
		var api *APIError
		if !errors.As(err, &api) || api.Code != "store_unavailable" {
			t.Fatal(path, err)
		}
	}
}
func TestClientRejectsBadResponses(t *testing.T) {
	for _, body := range []string{`{}`, `bad`, `{"apiVersion":"wrong"}`, `{"apiVersion":"ding.ing/v1alpha1","data":{}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); w.Write([]byte(body)) }))
		client := Client{URL: server.URL, Token: "secret", HTTP: server.Client()}
		if _, err := client.Call(context.Background(), "GET", "/", nil); err == nil {
			t.Fatal("accepted", body)
		}
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Error: &watch.Error{Code: "error", Message: "test"}})
	}))
	client := Client{URL: server.URL, HTTP: server.Client()}
	if _, err := client.Call(context.Background(), "GET", "/", nil); err == nil {
		t.Fatal("accepted error")
	}
	server.Close()
	if _, err := client.Call(context.Background(), "GET", "/", nil); err == nil {
		t.Fatal("dead server accepted")
	}
}
