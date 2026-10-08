package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/config"
)

func TestAuthenticatedRoles(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{AdminToken: "admin-token-long-enough", IngestToken: "ingest-token-long-enough", MaxBodyBytes: 1024}}
	s := New(lifecycleEngine(t), nil, cfg, "", nil, nil, nil)
	h, err := s.SecureHandler()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, token string
		want        int
	}{
		{"/health", "", 200}, {"/rules", "", 401}, {"/ingest", cfg.Server.AdminToken, 401}, {"/rules", cfg.Server.IngestToken, 401}, {"/rules", cfg.Server.AdminToken, 200}, {"/ingest", cfg.Server.IngestToken, 405},
	} {
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
}
func TestPrivateGeneratedCredentials(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ding", "tokens.json")
	c, err := loadCredentials(p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadCredentials(p)
	if err != nil || c != second || c.Admin == c.Ingest {
		t.Fatal("unstable credentials", err)
	}
	if err := os.WriteFile(p, []byte(`broken`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCredentials(p); err == nil {
		t.Fatal("silently replaced corrupt token file")
	}
}
