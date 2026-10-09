package control

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagedBackupCollectionOwnershipAndCleanup(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := newBackupManager(dir)
	mux := httpMuxForBackup(m, watchrun.New(s))
	call := func(method, path, owner string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Authorization", owner)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	create := func() BackupArtifact {
		t.Helper()
		w := call("POST", "/v1/console/backup", "owner")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var out struct{ Data BackupArtifact }
		if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Data
	}
	artifact := create()
	path := "/v1/console/backup/" + artifact.ID
	if w := call("GET", path, "other"); w.Code != 404 {
		t.Fatal("cross-owner access", w.Code)
	}
	if w := call("HEAD", path, "owner"); w.Code != 405 {
		t.Fatal("HEAD consumed artifact", w.Code)
	}
	if w := call("GET", path, "owner"); w.Code != 200 || !bytes.HasPrefix(w.Body.Bytes(), []byte("SQLite format 3")) {
		t.Fatal(w.Code, w.Body.Len())
	}
	if w := call("GET", path, "owner"); w.Code != 404 {
		t.Fatal("collected twice", w.Code)
	}
	if entries, _ := os.ReadDir(m.dir); len(entries) != 0 {
		t.Fatal("artifact leaked", entries)
	}
	if len(m.files) != 0 {
		t.Fatal("quota leaked")
	}
	expired := create()
	m.mu.Lock()
	f := m.files[expired.ID]
	f.ExpiresAt = time.Now().Add(-time.Second)
	m.files[expired.ID] = f
	m.mu.Unlock()
	if w := call("GET", "/v1/console/backup/"+expired.ID, "owner"); w.Code != 404 {
		t.Fatal(w.Code)
	}
	m.remove(expired.ID)
	stale := create()
	if _, err = os.Stat(filepath.Join(m.dir, stale.ID+".db")); err != nil {
		t.Fatal(err)
	}
	next := newBackupManager(dir)
	if next.err != nil {
		t.Fatal(next.err)
	}
	if entries, _ := os.ReadDir(m.dir); len(entries) != 0 {
		t.Fatal("restart artifact leak")
	}
	m.remove(stale.ID)
	// Active uncollected artifacts are bounded.
	one, two := create(), create()
	if w := call("POST", "/v1/console/backup", "owner"); w.Code != 429 {
		t.Fatal("unbounded artifacts", w.Code)
	}
	m.remove(one.ID)
	m.remove(two.ID)
}

func httpMuxForBackup(m *backupManager, a *watchrun.App) *http.ServeMux {
	mux := http.NewServeMux()
	m.routes(mux, a)
	return mux
}
func TestMigrationArchiveNeverAppliesAndReferenceIsBounded(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := watchrun.New(s)
	h := ConsoleHandler(app, Credentials{Admin: "admin"}, ConsoleConfig{StateDir: dir, Reference: func(topic string) (string, error) { return "help for " + topic, nil }})
	request := map[string]string{"manifest": `notifiers:
  console: {type: console}
rules:
  - name: hot
    condition: value > 5
    alert: [{notifier: console}]
`}
	body, _ := json.Marshal(request)
	r := httptest.NewRequest("POST", "/v1/tools/migrate", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct{ Data Conversion }
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(out.Data.Archive), int64(len(out.Data.Archive)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range archive.File {
		if f.Name == "report.json" {
			found = true
		}
		if strings.Contains(f.Name, "..") || filepath.IsAbs(f.Name) {
			t.Fatal(f.Name)
		}
	}
	if !found {
		t.Fatal("report omitted")
	}
	records, err := app.List(context.Background())
	if err != nil || len(records) != 0 {
		t.Fatal("migration applied watches", err)
	}
	r = httptest.NewRequest("GET", "/v1/reference?topic="+strings.Repeat("x", 121), nil)
	r.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
