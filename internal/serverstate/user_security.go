package state

import (
	"context"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/identity"
)

func (d *DB) ChangePassword(ctx context.Context, user, old, password string) error {
	d.Access.Lock()
	defer d.Access.Unlock()
	var hash string
	if err := d.sql.QueryRowContext(ctx, "SELECT password FROM users WHERE id=? AND disabled=0 AND password IS NOT NULL", user).Scan(&hash); err != nil {
		return fmt.Errorf("password unavailable")
	}
	valid, err := identity.VerifyPassword(hash, old)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("current password is incorrect")
	}
	hash, err = identity.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE users SET password=? WHERE id=?", hash, user); err != nil {
		return err
	}
	if err = revokeUser(ctx, tx, user); err != nil {
		return err
	}
	if err = audit(ctx, tx, user, "", "password:change", user); err != nil {
		return err
	}
	return tx.Commit()
}

type SessionInfo struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace"`
	Kind      string    `json:"kind"`
	ExpiresAt time.Time `json:"expiresAt"`
	Current   bool      `json:"current"`
}

func (d *DB) Sessions(ctx context.Context, user, token string) ([]SessionInfo, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT hash,workspace,kind,expires_at FROM sessions WHERE user=? AND expires_at>? ORDER BY expires_at DESC LIMIT 20", user, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		var s SessionInfo
		var at int64
		if err := rows.Scan(&s.ID, &s.Workspace, &s.Kind, &at); err != nil {
			return nil, err
		}
		s.ExpiresAt = time.Unix(at, 0)
		s.Current = s.ID == TokenHash(token)
		out = append(out, s)
	}
	return out, rows.Err()
}
func (d *DB) RevokeUserSession(ctx context.Context, user, id string) error {
	d.Access.Lock()
	defer d.Access.Unlock()
	_, err := d.sql.ExecContext(ctx, "DELETE FROM sessions WHERE user=? AND hash=?", user, id)
	return err
}

func (d *DB) DisableUser(ctx context.Context, actor access.Principal, user string, disabled bool) error {
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	actor, err = memberActor(ctx, tx, actor)
	if err != nil {
		return err
	}
	var role access.Role
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT role FROM memberships WHERE user=? AND workspace=?", user, actor.Workspace).Scan(&role); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM memberships WHERE user=?", user).Scan(&count); err != nil {
		return err
	}
	if count != 1 || ((role == access.Owner || role == access.Administrator) && !actor.Allows(access.Owners)) {
		return fmt.Errorf("host recovery required for this user")
	}
	if disabled && role == access.Owner {
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM memberships m JOIN users u ON u.id=m.user WHERE m.workspace=? AND m.user!=? AND m.role='owner' AND u.disabled=0 AND (u.password IS NOT NULL OR EXISTS(SELECT 1 FROM identities i WHERE i.user=u.id))`, actor.Workspace, user).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("assign another owner first")
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE users SET disabled=? WHERE id=?", disabled, user); err != nil {
		return err
	}
	if disabled {
		if err = revokeUser(ctx, tx, user); err != nil {
			return err
		}
	}
	if err = audit(ctx, tx, actor.User, actor.Workspace, "user:disabled", user); err != nil {
		return err
	}
	return tx.Commit()
}
