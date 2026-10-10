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
type Account struct {
	ID        string    `json:"id"`
	Issuer    string    `json:"-"`
	Subject   string    `json:"-"`
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
	if version > len(migrations) {
		return nil, fmt.Errorf("cloud control schema is newer than this binary")
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=100; PRAGMA journal_size_limit=4194304;"); err != nil {
		return nil, err
	}
	for version < len(migrations) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, migrations[version]); err != nil {
			return nil, err
		}
		version++
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	ok = true
	return &DB{db, unlock}, nil
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

func (d *DB) Account(ctx context.Context, id string) (Account, error) {
	return scanAccount(d.sql.QueryRowContext(ctx, "SELECT id,issuer,subject,created_at FROM accounts WHERE id=?", id))
}

type row interface{ Scan(...any) error }

func scanAccount(r row) (Account, error) {
	var a Account
	var created int64
	err := r.Scan(&a.ID, &a.Issuer, &a.Subject, &created)
	a.CreatedAt = time.Unix(created, 0).UTC()
	return a, err
}

// Enroll is called only after verifying issuer and immutable subject. A mutable
// login or email is never used to merge accounts or authorize a workspace.
func (d *DB) Enroll(ctx context.Context, issuer, subject string, cap int) (Account, error) {
	if issuer == "" || subject == "" || len(issuer) > 1024 || len(subject) > 512 || cap < 1 || cap > 1000 {
		return Account{}, fmt.Errorf("invalid enrollment")
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback()
	a, err := scanAccount(tx.QueryRowContext(ctx, "SELECT id,issuer,subject,created_at FROM accounts WHERE issuer=? AND subject=?", issuer, subject))
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return a, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts").Scan(&count); err != nil {
		return a, err
	}
	if count >= cap {
		return a, fmt.Errorf("cloud beta enrollment is full; local and self-hosted Ding remain available")
	}
	a = Account{ID: ID(), Issuer: issuer, Subject: subject, CreatedAt: time.Now().UTC()}
	if _, err := tx.ExecContext(ctx, "INSERT INTO accounts(id,issuer,subject,created_at) VALUES(?,?,?,?)", a.ID, a.Issuer, a.Subject, a.CreatedAt.Unix()); err != nil {
		return Account{}, err
	}
	return a, tx.Commit()
}

func (d *DB) Accounts(ctx context.Context) ([]Account, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT id,issuer,subject,created_at FROM accounts ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := []Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

var migrations = []string{`
CREATE TABLE accounts(id TEXT PRIMARY KEY,issuer TEXT NOT NULL,subject TEXT NOT NULL,created_at INTEGER NOT NULL,UNIQUE(issuer,subject));
CREATE TABLE secrets(account TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,name TEXT NOT NULL,ciphertext BLOB NOT NULL,PRIMARY KEY(account,name));
`, `
CREATE TABLE usage(account TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,period TEXT NOT NULL,checks INTEGER NOT NULL DEFAULT 0,deliveries INTEGER NOT NULL DEFAULT 0,bytes INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(account,period));
CREATE TABLE reservations(id TEXT PRIMARY KEY,account TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,period TEXT NOT NULL,created_at INTEGER NOT NULL);
CREATE INDEX reservations_account ON reservations(account);
`, `
CREATE TABLE sessions(hash TEXT PRIMARY KEY,account TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,csrf TEXT NOT NULL,kind TEXT NOT NULL,expires_at INTEGER NOT NULL);
CREATE INDEX sessions_account ON sessions(account);
CREATE TABLE logins(hash TEXT PRIMARY KEY,ciphertext BLOB NOT NULL,expires_at INTEGER NOT NULL);
`, `
CREATE TABLE probes(account TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,id TEXT NOT NULL,digest TEXT NOT NULL,outcome TEXT NOT NULL,at INTEGER NOT NULL,PRIMARY KEY(account,id));
`, `
CREATE TABLE devices(id TEXT PRIMARY KEY,challenge TEXT NOT NULL,account TEXT REFERENCES accounts(id) ON DELETE CASCADE,expires_at INTEGER NOT NULL);
`}
