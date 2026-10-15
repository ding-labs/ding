// Package store owns the single-writer local SQLite transaction boundary.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/runtimeperf"
	_ "modernc.org/sqlite"
)

const SchemaVersion = 12

//go:embed schema.sql
var initialSchema string

//go:embed schema2.sql
var inspectionSchema string

//go:embed schema3.sql
var queueIndexSchema string

//go:embed schema4.sql
var integrationSchema string

//go:embed schema5.sql
var handoffSchema string

//go:embed schema6.sql
var auditSchema string

//go:embed schema7.sql
var pushSchema string

//go:embed schema8.sql
var pushStatsSchema string

//go:embed schema9.sql
var deliveryStreamsSchema string

//go:embed schema10.sql
var entityPinsSchema string

//go:embed schema11.sql
var retentionIndexesSchema string

//go:embed schema12.sql
var retentionSweepIndexesSchema string
var ErrClosed = errors.New("store closed")
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("revision conflict")
var ErrStale = errors.New("stale generation")

// ErrQuota is shared by backend admission and the runtime API.
var ErrQuota = errors.New("resource quota exceeded")

type Store struct {
	ownership *database.Ownership
	postgres  bool
	workspace string
	mu        sync.RWMutex
	writer    chan struct{}
	db        *sql.DB
	readers   *sql.DB
	lock      *os.File
	dir       string
	closed    bool
}
type Tx struct {
	postgres bool
	changed  bool
	ctx      context.Context
	sql      *sql.Tx
	writable bool
}

func Open(ctx context.Context, dir string) (s *Store, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if _, e := os.Lstat(filepath.Join(dir, database.BindingFile)); e == nil {
		return nil, fmt.Errorf("server data directory cannot be used as personal watch state")
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	// Resolve symlinks so aliases share the OS lock file and SQLite URI.
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	lock, err := lockDirectory(filepath.Join(dir, "writer.lock"))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			unlockDirectory(lock)
		}
	}()
	path := filepath.Join(dir, "ding.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	uri := databaseURI(path)
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(FULL)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() {
		if err != nil {
			db.Close()
		}
	}()
	var mode string
	if err = db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		return nil, err
	}
	if mode != "wal" {
		return nil, fmt.Errorf("SQLite refused WAL mode")
	}
	if _, err = db.ExecContext(ctx, "PRAGMA wal_autocheckpoint=1000; PRAGMA journal_size_limit=16777216"); err != nil {
		return nil, err
	}
	s = &Store{db: db, lock: lock, dir: dir, writer: make(chan struct{}, 1)}
	if err = s.migrate(ctx, SchemaVersion, []string{initialSchema, inspectionSchema, queueIndexSchema, integrationSchema, handoffSchema, auditSchema, pushSchema, pushStatsSchema, deliveryStreamsSchema, entityPinsSchema, retentionIndexesSchema, retentionSweepIndexesSchema}); err != nil {
		return nil, err
	}
	s.readers, err = openSnapshotReaders(ctx, uri)
	if err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.postgres {
		return nil
	}
	readErr := s.readers.Close()
	_, checkpointErr := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	err := s.db.Close()
	unlockErr := unlockDirectory(s.lock)
	return errors.Join(readErr, checkpointErr, err, unlockErr)
}
func (s *Store) Update(ctx context.Context, fn func(*Tx) error) error {
	waited := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}
	select {
	case s.writer <- struct{}{}:
		defer func() { <-s.writer }()
	case <-ctx.Done():
		runtimeperf.Since(s.PerfBackend(), runtimeperf.WriterWait, waited)
		return ctx.Err()
	}
	runtimeperf.Since(s.PerfBackend(), runtimeperf.WriterWait, waited)
	if s.postgres {
		return s.updatePostgres(ctx, fn)
	}
	began := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	runtimeperf.Since(s.PerfBackend(), runtimeperf.Begin, began)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	check, err := database.LockTransactionGuard(ctx, tx, s.db)
	if err != nil {
		return err
	}
	p, attributed := access.FromContext(ctx)
	var before int64
	if attributed {
		if err := tx.QueryRowContext(ctx, "SELECT total_changes()").Scan(&before); err != nil {
			return err
		}
	}
	stx := &Tx{ctx: ctx, sql: tx, writable: true}
	callback := time.Now()
	err = fn(stx)
	runtimeperf.Since(s.PerfBackend(), runtimeperf.Callback, callback)
	if err != nil {
		return err
	}
	if attributed {
		var after int64
		if err := tx.QueryRowContext(ctx, "SELECT total_changes()").Scan(&after); err != nil {
			return err
		}
		if after > before {
			if err := stx.auditMutation(p); err != nil {
				return err
			}
		}
	}
	if err = check(); err != nil {
		return err
	}
	started := time.Now()
	err = tx.Commit()
	runtimeperf.Since(s.PerfBackend(), runtimeperf.Commit, started)
	return err
}
func (s *Store) View(ctx context.Context, fn func(*Tx) error) error {
	defer runtimeperf.Since(s.PerfBackend(), runtimeperf.Snapshot, time.Now())
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}
	if s.postgres {
		return s.viewPostgres(ctx, fn)
	}
	select {
	case snapshotSlots <- struct{}{}:
		defer func() { <-snapshotSlots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	tx, err := s.readers.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return fn(&Tx{ctx: ctx, sql: tx})
}
func (s *Store) migrate(ctx context.Context, target int, steps []string) error {
	var current int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > target {
		return fmt.Errorf("store schema %d is newer than supported %d; restore a compatible backup", current, target)
	}
	if current == target {
		return nil
	}
	if target > len(steps) {
		return fmt.Errorf("missing migration")
	}
	if current > 0 {
		if err := s.backup(ctx, filepath.Join(s.dir, fmt.Sprintf("before-schema-%d-%d.db", target, time.Now().UnixNano()))); err != nil {
			return fmt.Errorf("pre-migration backup: %w", err)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for version := current + 1; version <= target; version++ {
		if _, err := tx.ExecContext(ctx, steps[version-1]); err != nil {
			return fmt.Errorf("migration %d failed: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) Backup(ctx context.Context, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if s.postgres {
		return fmt.Errorf("PostgreSQL requires an instance-level backup")
	}
	return s.backup(ctx, path)
}
func (s *Store) backup(ctx context.Context, path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(path); err == nil {
		return fmt.Errorf("backup destination already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	// Private temporary output remains unadvertised until VACUUM and verification
	// succeed. Link publishes without replacing an existing backup.
	f, err := os.CreateTemp(filepath.Dir(path), ".ding-backup-*.db")
	if err != nil {
		return err
	}
	tmp := f.Name()
	f.Close()
	defer os.Remove(tmp)
	if _, err = s.db.ExecContext(ctx, "VACUUM INTO ?", tmp); err != nil {
		return err
	}
	check, err := sql.Open("sqlite", readOnlyURI(tmp))
	if err != nil {
		return err
	}
	var integrity string
	err = check.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity)
	check.Close()
	if err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("backup integrity check failed")
	}
	f, err = os.OpenFile(tmp, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Link(tmp, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

type Health struct {
	Backend       string `json:"backend"`
	Postgres      string `json:"postgres,omitempty"`
	Durability    string `json:"durability,omitempty"`
	LogicalBytes  int64  `json:"logicalBytes,omitempty"`
	SQLite        string `json:"sqlite"`
	Schema        int    `json:"schema"`
	Journal       string `json:"journal"`
	Synchronous   int    `json:"synchronous"`
	Integrity     string `json:"integrity"`
	DatabaseBytes int64  `json:"databaseBytes"`
	WALBytes      int64  `json:"walBytes"`
}

func (s *Store) Health(ctx context.Context) (Health, error) {
	if s.postgres {
		return s.healthPostgres(ctx)
	}
	h := Health{Backend: "sqlite"}
	err := s.View(ctx, func(tx *Tx) error {
		for _, q := range []struct {
			query  string
			target any
		}{{"SELECT sqlite_version()", &h.SQLite}, {"PRAGMA user_version", &h.Schema}, {"PRAGMA journal_mode", &h.Journal}, {"PRAGMA synchronous", &h.Synchronous}, {"PRAGMA quick_check", &h.Integrity}} {
			if err := tx.sql.QueryRowContext(ctx, q.query).Scan(q.target); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return h, err
	}
	for name, target := range map[string]*int64{"ding.db": &h.DatabaseBytes, "ding.db-wal": &h.WALBytes} {
		if info, err := os.Stat(filepath.Join(s.dir, name)); err == nil {
			*target = info.Size()
		}
	}
	return h, nil
}

func databaseURI(path string) url.URL {
	p := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return url.URL{Scheme: "file", Path: p}
}
func readOnlyURI(path string) string {
	uri := databaseURI(path)
	uri.RawQuery = "mode=ro"
	return uri.String()
}
