package state

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"github.com/ding-labs/ding/internal/access"
	"time"
)

// The browser receives only the request ID. Claiming the approved connection
// also requires the original device's random verifier, which never enters a URL.
func (d *DB) BeginDevice(ctx context.Context, challenge string, now time.Time) (string, error) {
	if len(challenge) != 64 {
		return "", fmt.Errorf("invalid device challenge")
	}
	tx, err := d.begin(ctx)
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
	return d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM devices WHERE id=? AND workspace IS NULL AND expires_at>?", id, now.Unix()).Scan(&n) == nil && n == 1
}

func (d *DB) ApproveDevice(ctx context.Context, id, user, workspace string, scopes []string, now time.Time) error {
	p, err := d.Principal(ctx, user, workspace, "browser", nil)
	if err != nil || len(scopes) == 0 {
		return fmt.Errorf("membership required")
	}
	for _, scope := range scopes {
		if !p.Allows(access.Action(scope)) {
			return fmt.Errorf("scope not permitted")
		}
	}
	raw, _ := json.Marshal(scopes)
	r, err := d.sql.ExecContext(ctx, "UPDATE devices SET user=?,workspace=?,scopes=? WHERE id=? AND workspace IS NULL AND expires_at>?", user, workspace, string(raw), id, now.Unix())
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
	tx, err := d.begin(ctx)
	if err != nil {
		return s, false, err
	}
	defer tx.Rollback()
	var challenge, user, workspace, raw string
	if err = tx.QueryRowContext(ctx, "SELECT challenge,COALESCE(user,''),COALESCE(workspace,''),COALESCE(scopes,'[]') FROM devices WHERE id=? AND expires_at>?", id, now.Unix()).Scan(&challenge, &user, &workspace, &raw); err != nil {
		return s, false, err
	}
	if subtle.ConstantTimeCompare([]byte(challenge), []byte(TokenHash(verifier))) != 1 {
		return s, false, fmt.Errorf("invalid device proof")
	}
	if workspace == "" {
		return s, false, nil
	}
	principal := access.Principal{User: user, Workspace: workspace, Kind: "browser"}
	if err = tx.QueryRowContext(ctx, `SELECT m.role FROM memberships m JOIN users u ON u.id=m.user JOIN workspaces w ON w.id=m.workspace WHERE m.user=? AND m.workspace=? AND u.disabled=0 AND w.deleting=0`, user, workspace).Scan(&principal.Role); err != nil {
		return s, false, fmt.Errorf("workspace is unavailable")
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=?", now.Unix()); err != nil {
		return s, false, err
	}
	var n int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions WHERE user=?", user).Scan(&n); err != nil {
		return s, false, err
	}
	if n >= 20 {
		return s, false, fmt.Errorf("too many active connections")
	}
	s = Session{User: user, Workspace: workspace, Kind: "device", Token: ID(), ExpiresAt: now.Add(7 * 24 * time.Hour)}
	if err = json.Unmarshal([]byte(raw), &s.Scopes); err != nil {
		return s, false, err
	}
	for _, scope := range s.Scopes {
		if !principal.Allows(access.Action(scope)) {
			return s, false, fmt.Errorf("permissions changed; approve a new connection")
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO sessions(hash,user,workspace,scopes,csrf,kind,expires_at) VALUES(?,?,?,?,?,?,?)", TokenHash(s.Token), user, workspace, raw, "", s.Kind, s.ExpiresAt.Unix()); err != nil {
		return s, false, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM devices WHERE id=?", id); err != nil {
		return s, false, err
	}
	return s, true, tx.Commit()
}
