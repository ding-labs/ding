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
	"time"

	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/mcpconfig"
	_ "modernc.org/sqlite"
)

type DB struct {
	sql    *sql.DB
	unlock func() error
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
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
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
	if version != 0 && version != schemaVersion {
		return nil, fmt.Errorf("incompatible server schema; initialize a fresh state directory")
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=100; PRAGMA journal_size_limit=4194304;"); err != nil {
		return nil, err
	}
	if version == 0 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	ok = true
	return &DB{sql: db, unlock: unlock}, nil
}

func (d *DB) Close() error {
	_, checkpoint := d.sql.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return errors.Join(checkpoint, d.sql.Close(), d.unlock())
}
func ID() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (d *DB) Workspace(ctx context.Context, id string) (Workspace, error) {
	return scanAccount(d.sql.QueryRowContext(ctx, "SELECT id,name,created_at FROM workspaces WHERE id=? AND deleting=0", id))
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
	rows, err := d.sql.QueryContext(ctx, "SELECT id,name,created_at FROM workspaces WHERE deleting=0 ORDER BY id")
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

const schemaVersion = 100

const schema = `
CREATE TABLE users(id TEXT PRIMARY KEY,name TEXT NOT NULL,username TEXT UNIQUE,password TEXT,disabled INTEGER NOT NULL DEFAULT 0,created_at INTEGER NOT NULL);
CREATE TABLE identities(issuer TEXT NOT NULL,subject TEXT NOT NULL,user TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,PRIMARY KEY(issuer,subject));
CREATE TABLE workspaces(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_at INTEGER NOT NULL,deleting INTEGER NOT NULL DEFAULT 0);
CREATE TABLE memberships(user TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,role TEXT NOT NULL CHECK(role IN ('viewer','editor','administrator','owner')),PRIMARY KEY(user,workspace));
CREATE TABLE secrets(workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,name TEXT NOT NULL,ciphertext BLOB NOT NULL,PRIMARY KEY(workspace,name));
CREATE TABLE usage(workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,period TEXT NOT NULL,checks INTEGER NOT NULL DEFAULT 0,deliveries INTEGER NOT NULL DEFAULT 0,bytes INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(workspace,period));
CREATE TABLE reservations(id TEXT PRIMARY KEY,workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,period TEXT NOT NULL,created_at INTEGER NOT NULL);
CREATE INDEX reservations_account ON reservations(workspace);
CREATE TABLE sessions(hash TEXT PRIMARY KEY,workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,csrf TEXT NOT NULL,kind TEXT NOT NULL,expires_at INTEGER NOT NULL);
CREATE INDEX sessions_account ON sessions(workspace);
CREATE TABLE logins(hash TEXT PRIMARY KEY,ciphertext BLOB NOT NULL,expires_at INTEGER NOT NULL);
CREATE TABLE probes(workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,id TEXT NOT NULL,digest TEXT NOT NULL,outcome TEXT NOT NULL,at INTEGER NOT NULL,PRIMARY KEY(workspace,id));
CREATE TABLE devices(id TEXT PRIMARY KEY,challenge TEXT NOT NULL,workspace TEXT REFERENCES workspaces(id) ON DELETE CASCADE,expires_at INTEGER NOT NULL);
CREATE TABLE mcp_bindings(workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,client TEXT NOT NULL,ciphertext BLOB NOT NULL,PRIMARY KEY(workspace,client));
CREATE TABLE handoff_proofs(workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,id TEXT NOT NULL,body BLOB NOT NULL,PRIMARY KEY(workspace,id));
`
