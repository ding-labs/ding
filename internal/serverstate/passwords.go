package state

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/identity"
)

func (d *DB) Bootstrap(ctx context.Context, username, name, password string) (Enrollment, error) {
	var out Enrollment
	username, err := identity.Username(username)
	if err != nil {
		return out, err
	}
	if strings.TrimSpace(name) == "" || len(name) > 100 {
		return out, fmt.Errorf("name must contain 1–100 bytes")
	}
	hash, err := identity.HashPassword(password)
	if err != nil {
		return out, err
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return out, err
	}
	if count != 0 {
		return out, fmt.Errorf("instance already initialized; use host recovery")
	}
	out = Enrollment{Workspace: Workspace{ID: ID(), Name: "Team", CreatedAt: time.Now().UTC()}, User: ID()}
	if _, err := tx.ExecContext(ctx, "INSERT INTO users(id,name,username,password,created_at) VALUES(?,?,?,?,?)", out.User, name, username, hash, out.CreatedAt.Unix()); err != nil {
		return out, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO workspaces(id,name,created_at) VALUES(?,?,?)", out.ID, out.Name, out.CreatedAt.Unix()); err != nil {
		return out, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO memberships(user,workspace,role) VALUES(?,?,'owner')", out.User, out.ID); err != nil {
		return out, err
	}
	if err := audit(ctx, tx, out.User, out.ID, "instance:bootstrap", out.User); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (d *DB) PasswordUser(ctx context.Context, username string) (User, string, error) {
	var u User
	var hash string
	err := d.sql.QueryRowContext(ctx, "SELECT id,name,username,password FROM users WHERE username=? AND password IS NOT NULL AND disabled=0", username).Scan(&u.ID, &u.Name, &u.Username, &hash)
	return u, hash, err
}

type Activation struct {
	User      string    `json:"user"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func activation(ctx context.Context, tx *transaction, user, workspace string) (Activation, error) {
	out := Activation{User: user, Token: ID(), ExpiresAt: time.Now().Add(time.Hour)}
	if _, err := tx.ExecContext(ctx, "DELETE FROM activations WHERE user=? OR expires_at<=?", user, time.Now().Unix()); err != nil {
		return out, err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO activations(hash,user,workspace,expires_at) VALUES(?,?,?,?)", TokenHash(out.Token), user, workspace, out.ExpiresAt.Unix())
	return out, err
}

func (d *DB) Invite(ctx context.Context, actor access.Principal, username, name string, role access.Role) (Activation, error) {
	var out Activation
	username, err := identity.Username(username)
	if err != nil {
		return out, err
	}
	if !role.Valid() || strings.TrimSpace(name) == "" || len(name) > 100 {
		return out, fmt.Errorf("valid name and role required")
	}
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	actor, err = memberActor(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	if (role == access.Owner || role == access.Administrator) && !actor.Allows(access.Owners) {
		return out, access.ErrDenied
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return out, err
	}
	if count >= 1000 {
		return out, fmt.Errorf("instance user limit reached")
	}
	user := ID()
	if _, err := tx.ExecContext(ctx, "INSERT INTO users(id,name,username,created_at) VALUES(?,?,?,?)", user, name, username, time.Now().Unix()); err != nil {
		return out, fmt.Errorf("username unavailable")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO memberships(user,workspace,role) VALUES(?,?,?)", user, actor.Workspace, role); err != nil {
		return out, err
	}
	out, err = activation(ctx, tx, user, actor.Workspace)
	if err != nil {
		return out, err
	}
	if err := audit(ctx, tx, actor.User, actor.Workspace, "user:invite", user); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (d *DB) ResetInvitation(ctx context.Context, actor access.Principal, user string) (Activation, error) {
	var out Activation
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	actor, err = memberActor(ctx, tx, actor)
	if err != nil {
		return out, err
	}
	var role access.Role
	var memberships int
	if err := tx.QueryRowContext(ctx, "SELECT role FROM memberships WHERE user=? AND workspace=?", user, actor.Workspace).Scan(&role); err != nil {
		return out, access.ErrDenied
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM memberships WHERE user=?", user).Scan(&memberships); err != nil {
		return out, err
	}
	if memberships != 1 || role == access.Owner || (role == access.Administrator && !actor.Allows(access.Owners)) {
		return out, fmt.Errorf("host recovery required for this user")
	}
	out, err = activation(ctx, tx, user, actor.Workspace)
	if err != nil {
		return out, err
	}
	if err := audit(ctx, tx, actor.User, actor.Workspace, "password:reset-issued", user); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (d *DB) Activate(ctx context.Context, token, password string) error {
	if len(token) != 64 {
		return fmt.Errorf("activation expired or unavailable")
	}
	hash, err := identity.HashPassword(password)
	if err != nil {
		return err
	}
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var user, workspace string
	if err := tx.QueryRowContext(ctx, `DELETE FROM activations WHERE hash=? AND expires_at>? AND user IN (SELECT id FROM users WHERE disabled=0) RETURNING user,workspace`, TokenHash(token), time.Now().Unix()).Scan(&user, &workspace); err != nil {
		return fmt.Errorf("activation expired or unavailable")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE users SET password=? WHERE id=?", hash, user); err != nil {
		return err
	}
	if err := revokeUser(ctx, tx, user); err != nil {
		return err
	}
	if err := audit(ctx, tx, user, workspace, "password:set", user); err != nil {
		return err
	}
	return tx.Commit()
}

func revokeUser(ctx context.Context, tx *transaction, user string) error {
	for _, table := range []string{"sessions", "devices", "activations", "mcp_bindings"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE user=?", user); err != nil {
			return err
		}
	}
	return nil
}

// RecoverPassword is only called by the offline host command while Open holds
// exclusive instance ownership. HTTP handlers must use ResetInvitation.
func (d *DB) RecoverPassword(ctx context.Context, username, password string) error {
	username, err := identity.Username(username)
	if err != nil {
		return err
	}
	hash, err := identity.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var user string
	if err := tx.QueryRowContext(ctx, "UPDATE users SET password=?,disabled=0 WHERE username=? RETURNING id", hash, username).Scan(&user); err != nil {
		return fmt.Errorf("user not found")
	}
	if err := revokeUser(ctx, tx, user); err != nil {
		return err
	}
	if err := audit(ctx, tx, "host-operator", "", "password:recover", user); err != nil {
		return err
	}
	return tx.Commit()
}
