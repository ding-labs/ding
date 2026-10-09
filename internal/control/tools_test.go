package control

import (
	"context"
	"encoding/json"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConsoleToolsArePureBoundedAndAuthenticated(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := watchrun.New(s)
	app.Lookup = func(string) (string, bool) { return "TOP_SECRET", true }
	h := ConsoleHandler(app, Credentials{Admin: "admin", Ingest: "ingest"}, ConsoleConfig{})
	call := func(path, body, token string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", "http://localhost"+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	raw, _ := json.Marshal(ToolRequest{Manifest: example})
	for _, token := range []string{"", "ingest"} {
		if w := call("/v1/tools/compile", string(raw), token); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	w := call("/v1/tools/compile", string(raw), "admin")
	if w.Code != 200 || strings.Contains(w.Body.String(), "TOP_SECRET") {
		t.Fatal(w.Code, w.Body.String())
	}
	var compiled struct{ Data CompileResult }
	if err = json.Unmarshal(w.Body.Bytes(), &compiled); err != nil || !compiled.Data.Valid || len(compiled.Data.Descriptions) != 1 {
		t.Fatal(err, w.Body.String())
	}
	rows, err := app.List(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("compile applied configuration", err)
	}
	bad, _ := json.Marshal(ToolRequest{Manifest: "kind: Nope"})
	w = call("/v1/tools/compile", string(bad), "admin")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "invalid_manifest") {
		t.Fatal(w.Code, w.Body.String())
	}
	huge, _ := json.Marshal(ToolRequest{Manifest: strings.Repeat("x", (1<<20)+1)})
	if w = call("/v1/tools/compile", string(huge), "admin"); w.Code != 413 {
		t.Fatal(w.Code)
	}
	for _, payload := range []string{`{"evidence":{}}`, `{"evidence":{"definition":{"metadata":{"id":"api"}},"event":{"id":"event"}}}`} {
		w = call("/v1/tools/replay", payload, "admin")
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		if !strings.Contains(w.Body.String(), "malformed") && !strings.Contains(w.Body.String(), "unavailable") {
			t.Fatal(w.Body.String())
		}
	}
	if w = call("/v1/tools/test", `{"manifest":"bad","fixture":""}`, "admin"); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
