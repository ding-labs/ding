package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/access"
)

type Member struct {
	User
	Role access.Role `json:"role"`
}

func (d *DB) Members(ctx context.Context, workspace string) ([]Member, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT u.id,u.name,COALESCE(u.username,''),u.disabled,m.role FROM memberships m JOIN users u ON u.id=m.user WHERE m.workspace=? ORDER BY u.name,u.id LIMIT 1000`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Name, &m.Username, &m.Disabled, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func memberActor(ctx context.Context, tx *sql.Tx, p access.Principal) (access.Principal, error) {
	err := tx.QueryRowContext(ctx, `SELECT m.role FROM memberships m JOIN users u ON u.id=m.user JOIN workspaces w ON w.id=m.workspace WHERE m.user=? AND m.workspace=? AND u.disabled=0 AND w.deleting=0`, p.User, p.Workspace).Scan(&p.Role)
	if err != nil || !p.Allows(access.Members) {
		return p, fmt.Errorf("member administration denied")
	}
	return p, nil
}

// ChangeMember uses an empty role to remove membership. The actor is resolved
// again under the same transaction as the last-owner check and mutation.
func (d *DB) ChangeMember(ctx context.Context, actor access.Principal, user string, role access.Role) error {
	if user == "" || (role != "" && !role.Valid()) {
		return fmt.Errorf("invalid member or role")
	}
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	actor, err = memberActor(ctx, tx, actor)
	if err != nil {
		return err
	}
	var old access.Role
	err = tx.QueryRowContext(ctx, "SELECT role FROM memberships WHERE user=? AND workspace=?", user, actor.Workspace).Scan(&old)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if (old == access.Owner || old == access.Administrator || role == access.Owner || role == access.Administrator) && !actor.Allows(access.Owners) {
		return fmt.Errorf("owner permission required")
	}
	if old == access.Owner && role != access.Owner {
		var owners int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM memberships m JOIN users u ON u.id=m.user WHERE workspace=? AND role='owner' AND u.disabled=0`, actor.Workspace).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return fmt.Errorf("assign another owner first")
		}
	}
	if role == "" {
		_, err = tx.ExecContext(ctx, "DELETE FROM memberships WHERE user=? AND workspace=?", user, actor.Workspace)
	} else {
		var active int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id=? AND disabled=0", user).Scan(&active); err != nil || active != 1 {
			return fmt.Errorf("user unavailable")
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO memberships(user,workspace,role) VALUES(?,?,?) ON CONFLICT(user,workspace) DO UPDATE SET role=excluded.role", user, actor.Workspace, role)
	}
	if err != nil {
		return err
	}
	if err := audit(ctx, tx, actor.User, actor.Workspace, "membership:"+string(role), user); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteUser removes only the authenticated user's identity, after ensuring
// every owned workspace has another enabled owner. Workspaces keep running.
func (d *DB) DeleteUser(ctx context.Context, user string) error {
	d.Access.Lock()
	defer d.Access.Unlock()
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sole int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM memberships mine WHERE mine.user=? AND mine.role='owner' AND NOT EXISTS(SELECT 1 FROM memberships other JOIN users u ON u.id=other.user WHERE other.workspace=mine.workspace AND other.user!=mine.user AND other.role='owner' AND u.disabled=0)`, user).Scan(&sole)
	if err != nil {
		return err
	}
	if sole != 0 {
		return fmt.Errorf("transfer workspace ownership first")
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM users WHERE id=?", user); err != nil {
		return err
	}
	if err = audit(ctx, tx, user, "", "user:delete", user); err != nil {
		return err
	}
	return tx.Commit()
}

type Audit struct {
	Sequence  int64     `json:"sequence"`
	Actor     string    `json:"actor"`
	Workspace string    `json:"workspace"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	At        time.Time `json:"at"`
}

func audit(ctx context.Context, tx *sql.Tx, actor, workspace, action, target string) error {
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit(actor,workspace,action,target,at) VALUES(?,?,?,?,?)", actor, workspace, action, target, time.Now().Unix()); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM audit WHERE sequence <= (SELECT COALESCE(MAX(sequence),0)-10000 FROM audit)")
	return err
}

func (d *DB) Audit(ctx context.Context, workspace string, before int64) ([]Audit, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT sequence,actor,workspace,action,target,at FROM audit WHERE workspace=? AND (?=0 OR sequence<?) ORDER BY sequence DESC LIMIT 100", workspace, before, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Audit{}
	for rows.Next() {
		var a Audit
		var at int64
		if err := rows.Scan(&a.Sequence, &a.Actor, &a.Workspace, &a.Action, &a.Target, &at); err != nil {
			return nil, err
		}
		a.At = time.Unix(at, 0).UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}
