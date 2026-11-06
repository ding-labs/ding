package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestTypedServicePreviewIsScopedOfflineAndStrict(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := watchrun.New(db)
	if _, err := a.Apply(ctx, watchrun.ApplyRequest{Manifest: "apiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: alerts}\nspec: {type: console}\n"}); err != nil {
		t.Fatal(err)
	}
	grant, err := a.CreateIntegrationGrant(ctx, watchrun.GrantRequest{Name: "preview", Days: 1, Scopes: []string{"inspect", "preview"}})
	if err != nil {
		t.Fatal(err)
	}
	read, err := a.CreateIntegrationGrant(ctx, watchrun.GrantRequest{Name: "read", Days: 1, Scopes: []string{"inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(204) }))
	defer source.Close()
	var output bytes.Buffer
	a.Output = &output
	h := Handler(a, Credentials{Admin: "admin", Ingest: "ingest"})
	call := func(token string, args map[string]any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(args)
		r := httptest.NewRequest("POST", "/v1/integrations/preview/service", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	args := map[string]any{"id": "service", "url": source.URL, "destinationRef": "alerts", "expectedStatus": 204}
	for _, token := range []string{"admin", "ingest", read.Token, ""} {
		if w := call(token, args); w.Code != 403 {
			t.Fatal("non-preview authority accepted", w.Code)
		}
	}
	w := call(grant.Token, args)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"handle"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"manifest"`)) || !bytes.Contains(w.Body.Bytes(), []byte("operator: ne")) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, change := range []map[string]any{{"expectedStatus": 600}, {"expectedStatus": 204.5}, {"destinationRef": "missing"}, {"id": ""}, {"url": "file:///etc/passwd"}, {"every": "1s"}, {"credential": "secret"}} {
		bad := map[string]any{}
		for key, value := range args {
			bad[key] = value
		}
		for key, value := range change {
			bad[key] = value
		}
		if w := call(grant.Token, bad); w.Code < 400 {
			t.Fatal("invalid starter accepted", change, w.Body.String())
		}
	}
	watches, err := a.List(ctx)
	if err != nil || len(watches) != 0 || requests.Load() != 0 || output.Len() != 0 {
		t.Fatal("preview ran a source, sent a notification, or created a watch", err)
	}
}
