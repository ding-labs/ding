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

	_ "modernc.org/sqlite"
)

const SchemaVersion = 4

//go:embed schema.sql
var initialSchema string

//go:embed schema2.sql
var inspectionSchema string

//go:embed schema3.sql
var queueIndexSchema string

//go:embed schema4.sql
var integrationSchema string
var ErrClosed = errors.New("store closed")
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("revision conflict")
var ErrStale = errors.New("stale generation")

type Store struct {
	mu     sync.Mutex
	db     *sql.DB
	lock   *os.File
	dir    string
	closed bool
}
type Tx struct {
	ctx context.Context
	sql *sql.Tx
}

func Open(ctx context.Context, dir string) (s *Store, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
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
	s = &Store{db: db, lock: lock, dir: dir}
	if err = s.migrate(ctx, SchemaVersion, []string{initialSchema, inspectionSchema, queueIndexSchema, integrationSchema}); err != nil {
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
	_, checkpointErr := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	err := s.db.Close()
	unlockErr := unlockDirectory(s.lock)
	return errors.Join(checkpointErr, err, unlockErr)
}
func (s *Store) Update(ctx context.Context, fn func(*Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(&Tx{ctx: ctx, sql: tx}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) View(ctx context.Context, fn func(*Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
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
	SQLite        string `json:"sqlite"`
	Schema        int    `json:"schema"`
	Journal       string `json:"journal"`
	Synchronous   int    `json:"synchronous"`
	Integrity     string `json:"integrity"`
	DatabaseBytes int64  `json:"databaseBytes"`
	WALBytes      int64  `json:"walBytes"`
}

func (s *Store) Health(ctx context.Context) (Health, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var h Health
	if s.closed {
		return h, ErrClosed
	}
	for _, q := range []struct {
		query  string
		target any
	}{{"SELECT sqlite_version()", &h.SQLite}, {"PRAGMA user_version", &h.Schema}, {"PRAGMA journal_mode", &h.Journal}, {"PRAGMA synchronous", &h.Synchronous}, {"PRAGMA quick_check", &h.Integrity}} {
		if err := s.db.QueryRowContext(ctx, q.query).Scan(q.target); err != nil {
			return h, err
		}
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
