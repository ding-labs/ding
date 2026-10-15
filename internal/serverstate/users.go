package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/access"
)

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
	Disabled bool   `json:"disabled"`
}

type Enrollment struct {
	Workspace
	User string
}

// Enroll is the cloud admission policy, invoked only with verified identity.
// Instance authentication never matches users by mutable names or email.
func (d *DB) Enroll(ctx context.Context, issuer, subject string, limit int) (Enrollment, error) {
	var out Enrollment
	if issuer == "" || subject == "" || len(issuer) > 1024 || len(subject) > 512 || limit < 0 || limit > 1000 {
		return out, fmt.Errorf("invalid enrollment")
	}
	if out, err := d.IdentityWorkspace(ctx, issuer, subject); err == nil {
		return out, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var existing string
	if err := tx.QueryRowContext(ctx, "SELECT user FROM identities WHERE issuer=? AND subject=?", issuer, subject).Scan(&existing); err == nil {
		return out, fmt.Errorf("user has no available workspace")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces").Scan(&count); err != nil {
		return out, err
	}
	if count >= limit {
		return out, fmt.Errorf("workspace enrollment is closed")
	}
	out = Enrollment{Workspace: Workspace{ID: ID(), Name: "Personal workspace", CreatedAt: time.Now().UTC()}, User: ID()}
	if _, err := tx.ExecContext(ctx, "INSERT INTO users(id,name,created_at) VALUES(?,?,?)", out.User, "Ding user", out.CreatedAt.Unix()); err != nil {
		return out, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO identities(issuer,subject,user) VALUES(?,?,?)", issuer, subject, out.User); err != nil {
		return out, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO workspaces(id,name,created_at) VALUES(?,?,?)", out.ID, out.Name, out.CreatedAt.Unix()); err != nil {
		return out, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO memberships(user,workspace,role) VALUES(?,?,?)", out.User, out.ID, access.Owner); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (d *DB) IdentityUser(ctx context.Context, issuer, subject string) (User, error) {
	var u User
	err := d.sql.QueryRowContext(ctx, `SELECT u.id,u.name,COALESCE(u.username,''),u.disabled FROM users u JOIN identities i ON i.user=u.id WHERE i.issuer=? AND i.subject=? AND u.disabled=0`, issuer, subject).Scan(&u.ID, &u.Name, &u.Username, &u.Disabled)
	return u, err
}

// IdentitySubject resolves only a previously verified identity at the pinned
// issuer. An email address or a caller-supplied provider user ID is never used.
func (d *DB) IdentitySubject(ctx context.Context, user, issuer string) (string, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT i.subject FROM identities i JOIN users u ON u.id=i.user WHERE i.user=? AND i.issuer=? AND u.disabled=0 LIMIT 2`, user, issuer)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var subject string
	n := 0
	for rows.Next() {
		if err := rows.Scan(&subject); err != nil {
			return "", err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if n != 1 {
		return "", fmt.Errorf("one verified provider identity is required")
	}
	return subject, nil
}

func (d *DB) IdentityWorkspace(ctx context.Context, issuer, subject string) (Enrollment, error) {
	var out Enrollment
	u, err := d.IdentityUser(ctx, issuer, subject)
	if err != nil {
		return out, err
	}
	out.User = u.ID
	var created int64
	err = d.sql.QueryRowContext(ctx, `SELECT w.id,w.name,w.created_at FROM workspaces w JOIN memberships m ON m.workspace=w.id WHERE m.user=? AND w.deleting=0 ORDER BY w.id LIMIT 1`, u.ID).Scan(&out.ID, &out.Name, &created)
	out.CreatedAt = time.Unix(created, 0).UTC()
	return out, err
}

func (d *DB) User(ctx context.Context, id string) (User, error) {
	var u User
	err := d.sql.QueryRowContext(ctx, "SELECT id,name,COALESCE(username,''),disabled FROM users WHERE id=?", id).Scan(&u.ID, &u.Name, &u.Username, &u.Disabled)
	return u, err
}

func (d *DB) Principal(ctx context.Context, user, workspace, kind string, scopes []string) (access.Principal, error) {
	p := access.Principal{User: user, Workspace: workspace, Kind: kind, Scopes: scopes}
	err := d.sql.QueryRowContext(ctx, `SELECT m.role FROM memberships m JOIN users u ON u.id=m.user JOIN workspaces w ON w.id=m.workspace WHERE m.user=? AND m.workspace=? AND u.disabled=0 AND w.deleting=0`, user, workspace).Scan(&p.Role)
	return p, err
}

type Membership struct {
	Workspace string      `json:"workspace"`
	Name      string      `json:"name"`
	Role      access.Role `json:"role"`
}

func (d *DB) Memberships(ctx context.Context, user string) ([]Membership, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT w.id,w.name,m.role FROM memberships m JOIN workspaces w ON w.id=m.workspace JOIN users u ON u.id=m.user WHERE m.user=? AND u.disabled=0 AND w.deleting=0 ORDER BY w.name,w.id LIMIT 100`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Membership{}
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.Workspace, &m.Name, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
