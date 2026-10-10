package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ding-labs/ding/internal/notify"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestFirstWatchPreviewRequiresAuthAndCannotReplaceExistingWatch(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := watchrun.New(s)
	calls := 0
	a.Notify = func(_ context.Context, m notify.Message) error {
		calls++
		if !m.RequestPermission {
			t.Fatal("missing explicit test permission")
		}
		return nil
	}
	h := ConsoleHandler(a, Credentials{Admin: "admin", Ingest: "ingest"}, ConsoleConfig{})
	call := func(path, token string, body any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://localhost"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	payload := map[string]string{"id": "first", "url": "http://127.0.0.1:3000/health", "delivery": "desktop"}
	for _, path := range []string{"/v1/onboarding/preview", "/v1/desktop/test"} {
		if w := call(path, "ingest", payload); w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("unauthorized notification")
	}
	w := call("/v1/onboarding/preview", "admin", payload)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct{ Data FirstWatchPreview }
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	rows, err := a.List(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("preview mutated state", err)
	}
	if w := call("/v1/apply", "admin", watchrun.ApplyRequest{Manifest: response.Data.Manifest, Review: response.Data.Review.Review}); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("/v1/onboarding/preview", "admin", payload); w.Code != 409 {
		t.Fatal("overwrote existing watch", w.Code)
	}
	if w := call("/v1/desktop/test", "admin", struct{}{}); w.Code != 200 || calls != 1 {
		t.Fatal(w.Code, calls)
	}
}
