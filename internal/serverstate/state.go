// Package state owns cloud identities and control metadata. The first deployment
// is deliberately a fenced single host, not a distributed expiring-lease system.
package state

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/mcpconfig"
	_ "modernc.org/sqlite"
)

type DB struct {
	ownership *database.Ownership
	postgres  bool
	sql       *sql.DB
	readers   *sql.DB
	unlock    func() error
	// Access serializes permission changes with admitted runtime operations.
	// Request adapters hold RLock until the operation completes; administration
	// methods take Lock before rechecking permissions in their transaction.
	Access sync.RWMutex
}
type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func Open(ctx context.Context, dir string) (*DB, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	unlock, err := install.Lock(dir)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			unlock()
		}
	}()
	path, err := filepath.Abs(filepath.Join(dir, "control.db"))
	if err != nil {
		return nil, err
	}
	f, err := mcpconfig.CreatePrivate(path)
	if err == nil {
		err = f.Close()
	} else if errors.Is(err, os.ErrExist) {
		err = mcpconfig.CheckPrivate(path)
	}
	if err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(path)
	// Windows drive letters belong in the URI path, not its authority.
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(FULL)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return nil, err
	}
	if version != 0 && (version < 100 || version > schemaVersion) {
		return nil, fmt.Errorf("incompatible server schema; use a compatible binary or backup")
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=100; PRAGMA journal_size_limit=4194304;"); err != nil {
		return nil, err
	}
	if version < schemaVersion {
		if version > 0 {
			backup := filepath.Join(dir, fmt.Sprintf("before-schema-%d-%d.db", schemaVersion, time.Now().UnixNano()))
			if err := (&DB{sql: db}).Backup(ctx, backup); err != nil {
				return nil, fmt.Errorf("pre-migration control backup: %w", err)
			}
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if version == 0 {
			if _, err := tx.ExecContext(ctx, schema); err != nil {
				return nil, err
			}
			version = 100
		}
		if version == 100 {
			if _, err := tx.ExecContext(ctx, serviceKeyMigration); err != nil {
				return nil, fmt.Errorf("server migration 101: %w", err)
			}
		}
		if version <= 101 {
			if _, err := tx.ExecContext(ctx, webhookMigration); err != nil {
				return nil, fmt.Errorf("server migration 102: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	readers, err := openControlReaders(ctx, u)
	if err != nil {
		return nil, err
	}
	ok = true
	return &DB{sql: db, readers: readers, unlock: unlock}, nil
}

func (d *DB) Close() error {
	if d.postgres {
		return nil
	}
	var readErr error
	if d.readers != nil {
		readErr = d.readers.Close()
	}
	_, checkpoint := d.sql.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return errors.Join(readErr, checkpoint, d.sql.Close(), d.unlock())
}

func openControlReaders(ctx context.Context, uri url.URL) (*sql.DB, error) {
	q := uri.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "cache_size(-256)")
	uri.RawQuery = q.Encode()
	readers, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	readers.SetMaxOpenConns(4)
	readers.SetMaxIdleConns(1)
	readers.SetConnMaxIdleTime(time.Minute)
	if err := readers.PingContext(ctx); err != nil {
		readers.Close()
		return nil, err
	}
	return readers, nil
}

func (d *DB) credentialDB() *sql.DB {
	if d.readers != nil {
		return d.readers
	}
	return d.sql
}
func ID() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (d *DB) Workspace(ctx context.Context, id string) (Workspace, error) {
	return scanAccount(d.sql.QueryRowContext(ctx, d.query(qWorkspace1), id))
}

type row interface{ Scan(...any) error }

func scanAccount(r row) (Workspace, error) {
	var a Workspace
	var created int64
	err := r.Scan(&a.ID, &a.Name, &created)
	a.CreatedAt = time.Unix(created, 0).UTC()
	return a, err
}

func (d *DB) Workspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := d.sql.QueryContext(ctx, d.query(qWorkspaces1))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	workspaces := []Workspace{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, a)
	}
	return workspaces, rows.Err()
}

const schemaVersion = 102

const schema = `
CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE users(id TEXT PRIMARY KEY,name TEXT NOT NULL,username TEXT UNIQUE,password TEXT,disabled INTEGER NOT NULL DEFAULT 0,created_at INTEGER NOT NULL);
CREATE TABLE identities(issuer TEXT NOT NULL,subject TEXT NOT NULL,user TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,PRIMARY KEY(issuer,subject));
CREATE TABLE workspaces(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_at INTEGER NOT NULL,deleting INTEGER NOT NULL DEFAULT 0);
CREATE TABLE memberships(user TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,role TEXT NOT NULL CHECK(role IN ('viewer','editor','administrator','owner')),PRIMARY KEY(user,workspace));
CREATE TABLE audit(sequence INTEGER PRIMARY KEY AUTOINCREMENT,actor TEXT NOT NULL,workspace TEXT NOT NULL,action TEXT NOT NULL,target TEXT NOT NULL,at INTEGER NOT NULL);
CREATE TABLE service_grants(id TEXT PRIMARY KEY,hash TEXT UNIQUE NOT NULL,name TEXT NOT NULL,workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,created_by TEXT NOT NULL,scopes TEXT NOT NULL,watches TEXT NOT NULL,expires_at INTEGER NOT NULL);
CREATE TABLE activations(hash TEXT PRIMARY KEY,user TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,expires_at INTEGER NOT NULL);
CREATE TABLE secrets(workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,name TEXT NOT NULL,ciphertext BLOB NOT NULL,PRIMARY KEY(workspace,name));
CREATE TABLE usage(workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,period TEXT NOT NULL,checks INTEGER NOT NULL DEFAULT 0,deliveries INTEGER NOT NULL DEFAULT 0,bytes INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(workspace,period));
CREATE TABLE reservations(id TEXT PRIMARY KEY,workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,period TEXT NOT NULL,created_at INTEGER NOT NULL);
CREATE INDEX reservations_account ON reservations(workspace);
CREATE TABLE sessions(hash TEXT PRIMARY KEY,user TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,scopes TEXT NOT NULL,workspace TEXT NOT NULL,csrf TEXT NOT NULL,kind TEXT NOT NULL,expires_at INTEGER NOT NULL);
CREATE INDEX sessions_account ON sessions(workspace);
CREATE TABLE logins(hash TEXT PRIMARY KEY,ciphertext BLOB NOT NULL,expires_at INTEGER NOT NULL);
CREATE TABLE probes(workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,id TEXT NOT NULL,digest TEXT NOT NULL,outcome TEXT NOT NULL,at INTEGER NOT NULL,PRIMARY KEY(workspace,id));
CREATE TABLE devices(id TEXT PRIMARY KEY,user TEXT REFERENCES users(id) ON DELETE CASCADE,scopes TEXT,challenge TEXT NOT NULL,workspace TEXT REFERENCES workspaces(id) ON DELETE CASCADE,expires_at INTEGER NOT NULL);
CREATE TABLE mcp_bindings(user TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,client TEXT NOT NULL,credential_hash TEXT NOT NULL UNIQUE,ciphertext BLOB NOT NULL,PRIMARY KEY(user,workspace,client));
CREATE TABLE handoff_proofs(workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,id TEXT NOT NULL,body BLOB NOT NULL,PRIMARY KEY(workspace,id));
`

const serviceKeyMigration = `
ALTER TABLE service_grants ADD COLUMN lifetimes TEXT NOT NULL DEFAULT '{}';
CREATE TABLE service_keys(id TEXT PRIMARY KEY,service TEXT NOT NULL REFERENCES service_grants(id) ON DELETE CASCADE,hash TEXT UNIQUE NOT NULL,expires_at INTEGER NOT NULL,created_at INTEGER NOT NULL,last_used_at INTEGER NOT NULL DEFAULT 0);
CREATE INDEX service_keys_service ON service_keys(service);
INSERT INTO service_keys(id,service,hash,expires_at,created_at) SELECT id,id,hash,expires_at,0 FROM service_grants;
`

const webhookMigration = `CREATE TABLE webhook_connections(id TEXT PRIMARY KEY,workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,lifetime TEXT NOT NULL,body BLOB NOT NULL,secret BLOB NOT NULL,previous BLOB,previous_until INTEGER NOT NULL);`
