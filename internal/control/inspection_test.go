package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestInspectionAPIContracts(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := watchrun.New(s)
	app.Now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	creds := Credentials{Admin: strings.Repeat("a", 64), Ingest: strings.Repeat("b", 64)}
	server := httptest.NewServer(Handler(app, creds))
	defer server.Close()
	c := Client{URL: server.URL, Token: creds.Admin, HTTP: server.Client()}
	call := func(method, path string, body any) json.RawMessage {
		t.Helper()
		raw, err := c.Call(ctx, method, path, body)
		if err != nil {
			t.Fatal(path, err)
		}
		return raw
	}
	call("POST", "/v1/apply", watchrun.ApplyRequest{Manifest: example})
	for _, path := range []string{"/v1/doctor", "/v1/watches/api/export", "/v1/events?watch=api"} {
		if !json.Valid(call("GET", path, nil)) {
			t.Fatal(path)
		}
	}
	var page store.EventPage
	json.Unmarshal(call("GET", "/v1/events?watch=api", nil), &page)
	event := page.Events[0]
	call("GET", "/v1/events/"+event.ID, nil)
	call("GET", "/v1/events/"+event.ID+"/observations", nil)
	call("POST", "/v1/backup", map[string]string{"path": filepath.Join(t.TempDir(), "backup.db")})
	for _, tc := range []struct {
		method, path, code string
		body               any
	}{
		{"GET", "/v1/events?limit=0", "invalid_cursor", nil}, {"GET", "/v1/events?limit=no", "invalid_cursor", nil}, {"GET", "/v1/events?watch=other&cursor=" + page.Cursor, "invalid_cursor", nil},
		{"GET", "/v1/events/missing", "not_found", nil}, {"GET", "/v1/events/missing/observations", "not_found", nil}, {"GET", "/v1/events/id/observations?after=-1", "invalid_cursor", nil},
		{"GET", "/v1/deliveries/-1", "invalid_request", nil}, {"GET", "/v1/deliveries/1?before=-1", "invalid_request", nil}, {"GET", "/v1/deliveries/999", "not_found", nil},
		{"POST", "/v1/deliveries/no/retry", "invalid_request", nil}, {"POST", "/v1/deliveries/999/retry", "not_found", nil},
		{"POST", "/v1/backup", "invalid_request", map[string]string{"path": "relative"}}, {"GET", "/v1/unknown", "not_found", nil}, {"DELETE", "/v1/events", "not_found", nil},
	} {
		_, err := c.Call(ctx, tc.method, tc.path, tc.body)
		var api *APIError
		if !errors.As(err, &api) || api.Code != tc.code {
			t.Fatal(tc, err)
		}
	}
	ingest := c
	ingest.Token = creds.Ingest
	for _, path := range []string{"/v1/events", "/v1/doctor", "/v1/watches/api/export"} {
		if _, err := ingest.Call(ctx, "GET", path, nil); err == nil {
			t.Fatal("ingest credential read admin data", path)
		}
	}
	// Cursor before deletion must report HTTP 410 via its stable error code.
	call("POST", "/v1/watches/api/lifecycle", watchrun.LifecycleRequest{Action: "pause"})
	if err := app.Maintain(ctx, app.Now().Add(8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, err = c.Call(ctx, "GET", "/v1/events?watch=api&cursor="+page.Cursor, nil)
	var api *APIError
	if !errors.As(err, &api) || api.Code != "cursor_expired" {
		t.Fatal(err)
	}
	s.Close()
	_, err = c.Call(ctx, "GET", "/v1/doctor", nil)
	if !errors.As(err, &api) || api.Code != "store_unavailable" {
		t.Fatal(err)
	}
}
