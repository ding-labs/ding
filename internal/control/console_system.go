package control

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/migrate"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

type ConvertedFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}
type Conversion struct {
	Report  migrate.Report  `json:"report"`
	Files   []ConvertedFile `json:"files"`
	Archive []byte          `json:"archive"`
}
type BackupArtifact struct {
	ID        string    `json:"id"`
	Bytes     int64     `json:"bytes"`
	ExpiresAt time.Time `json:"expiresAt"`
	Verified  bool      `json:"verified"`
}
type backupFile struct {
	BackupArtifact
	owner string
	path  string
	timer *time.Timer
}
type backupManager struct {
	mu     sync.Mutex
	files  map[string]backupFile
	dir    string
	err    error
	active bool
}

var backupName = regexp.MustCompile(`^[0-9a-f]{64}\.db$`)

const backupLimit = 512 << 20

func newBackupManager(state string) *backupManager {
	m := &backupManager{files: map[string]backupFile{}}
	if state == "" {
		m.err = fmt.Errorf("state directory unavailable")
		return m
	}
	m.dir = filepath.Join(state, "console-downloads")
	// Only our private artifact directory and exact generated file names are managed.
	if info, err := os.Lstat(m.dir); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		m.err = fmt.Errorf("invalid artifact directory")
		return m
	}
	if m.err = os.MkdirAll(m.dir, 0700); m.err != nil {
		return m
	}
	if m.err = os.Chmod(m.dir, 0700); m.err != nil {
		return m
	}
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		m.err = err
		return m
	}
	for _, e := range entries {
		if !e.IsDir() && (backupName.MatchString(e.Name()) || regexp.MustCompile(`^\.ding-backup-.*\.db$`).MatchString(e.Name())) {
			if err = os.Remove(filepath.Join(m.dir, e.Name())); err != nil {
				m.err = err
				return m
			}
		}
	}
	return m
}
func artifactOwner(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if session, ok := r.Context().Value(browserOwnerKey{}).(string); ok {
		value = session
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}
func (m *backupManager) remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.files[id]; ok {
		delete(m.files, id)
		if f.timer != nil {
			f.timer.Stop()
		}
		_ = os.Remove(f.path)
	}
}
func (m *backupManager) routes(mux *http.ServeMux, app *watchrun.App) {
	mux.HandleFunc("POST /v1/console/backup", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		if m.err != nil {
			m.mu.Unlock()
			fail(w, 503, "backup_unavailable", "managed artifact storage is unavailable")
			return
		}
		if m.active || len(m.files) >= 2 {
			m.mu.Unlock()
			fail(w, 429, "backup_busy", "a backup is running or two downloads are awaiting collection")
			return
		}
		m.active = true
		m.mu.Unlock()
		defer func() { m.mu.Lock(); m.active = false; m.mu.Unlock() }()
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		var usage store.Usage
		err := app.Store.View(ctx, func(tx *store.Tx) error { var err error; usage, err = tx.Budget(); return err })
		if err != nil {
			inspectionError(w, err)
			return
		}
		if usage.Bytes > backupLimit {
			fail(w, 413, "backup_too_large", "browser backup limit is 512 MiB; use Save on daemon host")
			return
		}
		id := randomToken()
		path := filepath.Join(m.dir, id+".db")
		if err = app.Store.Backup(ctx, path); err != nil {
			_ = os.Remove(path)
			fail(w, 503, "backup_failed", "verified backup could not be created")
			return
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() > backupLimit {
			_ = os.Remove(path)
			fail(w, 413, "backup_too_large", "browser backup limit is 512 MiB; use Save on daemon host")
			return
		}
		f := backupFile{BackupArtifact: BackupArtifact{ID: id, Bytes: info.Size(), ExpiresAt: time.Now().UTC().Add(5 * time.Minute), Verified: true}, owner: artifactOwner(r), path: path}
		if ctx.Err() != nil {
			_ = os.Remove(path)
			return
		}
		m.mu.Lock()
		f.timer = time.AfterFunc(5*time.Minute, func() { m.remove(id) })
		m.files[id] = f
		m.mu.Unlock()
		write(w, 200, f.BackupArtifact, nil)
	})
	mux.HandleFunc("GET /v1/console/backup/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			fail(w, 405, "method_not_allowed", "use GET to collect a backup")
			return
		}
		id := r.PathValue("id")
		m.mu.Lock()
		f, ok := m.files[id]
		if !ok || f.owner != artifactOwner(r) || time.Now().After(f.ExpiresAt) {
			m.mu.Unlock()
			fail(w, 404, "artifact_unavailable", "download expired, was collected, or belongs to another session")
			return
		}
		file, err := os.Open(f.path)
		if err != nil {
			m.mu.Unlock()
			m.remove(id)
			fail(w, 404, "artifact_unavailable", "download is unavailable")
			return
		}
		claimed := f
		claimed.owner = "collected"
		m.files[id] = claimed
		f.timer.Stop()
		m.mu.Unlock()
		defer func() { file.Close(); m.remove(id) }()
		w.Header().Set("Content-Type", "application/vnd.sqlite3")
		w.Header().Set("Content-Disposition", `attachment; filename="ding-backup.db"`)
		w.Header().Set("Content-Length", fmt.Sprint(f.Bytes))
		http.ServeContent(w, r, "ding-backup.db", time.Time{}, file)
	})
}
func systemRoutes(mux *http.ServeMux, app *watchrun.App, cfg ConsoleConfig) {
	newBackupManager(cfg.StateDir).routes(mux, app)
	diagnostics := make(chan struct{}, 1)
	mux.HandleFunc("GET /v1/console/doctor", func(w http.ResponseWriter, r *http.Request) {
		select {
		case diagnostics <- struct{}{}:
			defer func() { <-diagnostics }()
		default:
			fail(w, 429, "doctor_busy", "diagnostics are already running")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		d, err := app.Doctor(ctx)
		if err != nil {
			inspectionError(w, err)
			return
		}
		write(w, 200, d, nil)
	})
	mux.HandleFunc("GET /v1/reference", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Reference == nil {
			fail(w, 503, "reference_unavailable", "command reference is unavailable in this embedding")
			return
		}
		topic := r.URL.Query().Get("topic")
		if len(topic) > 120 {
			fail(w, 400, "invalid_topic", "unknown command")
			return
		}
		text, err := cfg.Reference(topic)
		if err != nil {
			fail(w, 400, "invalid_topic", err.Error())
			return
		}
		write(w, 200, map[string]string{"topic": topic, "text": text}, nil)
	})
	migrationSlots := make(chan struct{}, 1)
	mux.HandleFunc("POST /v1/tools/migrate", func(w http.ResponseWriter, r *http.Request) {
		select {
		case migrationSlots <- struct{}{}:
			defer func() { <-migrationSlots }()
		default:
			fail(w, 429, "tool_busy", "legacy conversion is already running")
			return
		}
		var req struct {
			Manifest string `json:"manifest"`
		}
		if decodeBounded(w, r, &req, 2<<20) != nil {
			fail(w, 400, "invalid_request", "expected legacy YAML, at most 1 MiB")
			return
		}
		result, err := migrate.Convert([]byte(req.Manifest))
		if err != nil {
			fail(w, 400, "invalid_legacy", err.Error())
			return
		}
		out := Conversion{Report: result.Report, Files: []ConvertedFile{}}
		var buffer bytes.Buffer
		archive := zip.NewWriter(&buffer)
		for _, f := range result.Files {
			out.Files = append(out.Files, ConvertedFile{Name: f.Name, Content: string(f.Content)})
			entry, err := archive.Create(f.Name)
			if err != nil {
				fail(w, 500, "archive_failed", "could not create conversion archive")
				return
			}
			if _, err = entry.Write(f.Content); err != nil {
				fail(w, 500, "archive_failed", "could not create conversion archive")
				return
			}
		}
		report, _ := json.MarshalIndent(result.Report, "", "  ")
		entry, err := archive.Create("report.json")
		if err == nil {
			_, err = entry.Write(report)
		}
		if err != nil || archive.Close() != nil {
			fail(w, 500, "archive_failed", "could not create conversion archive")
			return
		}
		out.Archive = buffer.Bytes()
		write(w, 200, out, nil)
	})
}
