package state

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"
)

// The browser receives only the request ID. Claiming the approved connection
// also requires the original device's random verifier, which never enters a URL.
func (d *DB) BeginDevice(ctx context.Context, challenge string, now time.Time) (string, error) {
	if len(challenge) != 64 {
		return "", fmt.Errorf("invalid device challenge")
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM devices WHERE expires_at<=?", now.Unix()); err != nil {
		return "", err
	}
	var n int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM devices").Scan(&n); err != nil {
		return "", err
	}
	if n >= 1000 {
		return "", fmt.Errorf("device connection capacity reached")
	}
	id := ID()
	if _, err = tx.ExecContext(ctx, "INSERT INTO devices(id,challenge,expires_at) VALUES(?,?,?)", id, challenge, now.Add(10*time.Minute).Unix()); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func (d *DB) DevicePending(ctx context.Context, id string, now time.Time) bool {
	var n int
	return d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM devices WHERE id=? AND account IS NULL AND expires_at>?", id, now.Unix()).Scan(&n) == nil && n == 1
}

func (d *DB) ApproveDevice(ctx context.Context, id, account string, now time.Time) error {
	r, err := d.sql.ExecContext(ctx, "UPDATE devices SET account=? WHERE id=? AND account IS NULL AND expires_at>? AND EXISTS(SELECT 1 FROM accounts WHERE id=? AND deleting=0)", account, id, now.Unix(), account)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("device request expired or already approved")
	}
	return nil
}

// ClaimDevice atomically consumes approval and issues a distinct, revocable
// session. A lost claim response requires a new approval; it cannot be replayed.
func (d *DB) ClaimDevice(ctx context.Context, id, verifier string, now time.Time) (Session, bool, error) {
	var s Session
	if len(id) != 64 || len(verifier) != 64 {
		return s, false, fmt.Errorf("invalid device proof")
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return s, false, err
	}
	defer tx.Rollback()
	var challenge, account string
	if err = tx.QueryRowContext(ctx, "SELECT challenge,COALESCE(account,'') FROM devices WHERE id=? AND expires_at>?", id, now.Unix()).Scan(&challenge, &account); err != nil {
		return s, false, err
	}
	if subtle.ConstantTimeCompare([]byte(challenge), []byte(TokenHash(verifier))) != 1 {
		return s, false, fmt.Errorf("invalid device proof")
	}
	if account == "" {
		return s, false, nil
	}
	var active int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts WHERE id=? AND deleting=0", account).Scan(&active); err != nil || active != 1 {
		return s, false, fmt.Errorf("workspace is unavailable")
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=?", now.Unix()); err != nil {
		return s, false, err
	}
	var n int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions WHERE account=?", account).Scan(&n); err != nil {
		return s, false, err
	}
	if n >= 20 {
		return s, false, fmt.Errorf("too many active connections")
	}
	s = Session{Account: account, Kind: "device", Token: ID(), ExpiresAt: now.Add(7 * 24 * time.Hour)}
	if _, err = tx.ExecContext(ctx, "INSERT INTO sessions(hash,account,csrf,kind,expires_at) VALUES(?,?,?,?,?)", TokenHash(s.Token), account, "", s.Kind, s.ExpiresAt.Unix()); err != nil {
		return s, false, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM devices WHERE id=?", id); err != nil {
		return s, false, err
	}
	return s, true, tx.Commit()
}
