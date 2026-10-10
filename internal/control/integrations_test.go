package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestIntegrationRouteBoundaryAndFullReview(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := watchrun.New(s)
	c := Credentials{Admin: strings.Repeat("a", 64), Ingest: strings.Repeat("i", 64)}
	for _, handler := range []http.Handler{Handler(a, c), ConsoleHandler(a, c, ConsoleConfig{})} {
		request := func(method, path, token string, body any, origin string) (int, json.RawMessage) {
			raw, _ := json.Marshal(body)
			r := httptest.NewRequest(method, path, bytes.NewReader(raw))
			r.Header.Set("Authorization", "Bearer "+token)
			if origin != "" {
				r.Header.Set("Origin", origin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			var e struct {
				Data json.RawMessage `json:"data"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &e)
			return w.Code, e.Data
		}
		status, raw := request("POST", "/v1/integrations/grants", c.Admin, watchrun.GrantRequest{Name: "Claude", Scopes: []string{"inspect", "preview", "manage"}, Days: 1}, "")
		if status != 201 {
			t.Fatal("pairing", status)
		}
		var paired watchrun.GrantPairing
		if err = json.Unmarshal(raw, &paired); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/v1/watches", "/v1/info", "/v1/integrations/grants"} {
			code, _ := request("GET", path, paired.Token, nil, "")
			if code != 401 && code != 403 {
				t.Fatal("escaped integration boundary", path, code)
			}
		}
		for _, path := range []string{"/v1/apply", "/v1/browser/handoff", "/v1/backup", "/v1/integrations/grants"} {
			code, _ := request("POST", path, paired.Token, map[string]any{}, "")
			if code != 401 && code != 403 {
				t.Fatal("privileged route", path, code)
			}
		}
		if code, _ := request("GET", "/v1/integrations/watches", paired.Token, nil, "https://attacker.example"); code != 403 {
			t.Fatal("browser origin accepted")
		}
		if code, _ := request("GET", "/v1/integrations/capabilities", c.Admin, nil, ""); code != 403 {
			t.Fatal("admin used as integration token")
		}
		if code, _ := request("GET", "/v1/integrations/watches/api%2Fexport", paired.Token, nil, ""); code != 400 {
			t.Fatal("encoded ID escaped fixed route", code)
		}
		status, raw = request("POST", "/v1/integrations/preview", paired.Token, map[string]string{"manifest": example}, "")
		if status != 200 {
			t.Fatal("preview", status)
		}
		var preview struct {
			Preview watchrun.IntegrationPreview `json:"preview"`
		}
		if err = json.Unmarshal(raw, &preview); err != nil || preview.Preview.Handle == "" {
			t.Fatal(string(raw), err)
		}
		body := map[string]string{"handle": preview.Preview.Handle, "operationKey": "api_operation_000001"}
		status, raw = request("POST", "/v1/integrations/apply", paired.Token, body, "")
		if status != 200 {
			t.Fatal("apply", status)
		}
		code, again := request("POST", "/v1/integrations/apply", paired.Token, body, "")
		if code != 200 || !bytes.Equal(raw, again) {
			t.Fatal("lost response retry changed result", code)
		}
		for _, path := range []string{"/v1/integrations/watches", "/v1/integrations/watches/api", "/v1/integrations/events", "/v1/integrations/deliveries", "/v1/integrations/destinations"} {
			code, raw := request("GET", path, paired.Token, nil, "")
			if code != 200 || !json.Valid(raw) {
				t.Fatal("read", path, code)
			}
		}
		code, _ = request("DELETE", "/v1/integrations/grants/"+paired.Grant.ID, c.Admin, nil, "")
		if code != 200 {
			t.Fatal(code)
		}
		code, _ = request("GET", "/v1/integrations/watches", paired.Token, nil, "")
		if code != 403 {
			t.Fatal("revoked grant read", code)
		}
	}
}
