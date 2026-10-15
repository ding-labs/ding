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
	rows, err := d.sql.QueryContext(ctx, d.query(qMembers1), workspace)
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

func memberActor(ctx context.Context, tx *transaction, p access.Principal) (access.Principal, error) {
	err := tx.QueryRowContext(ctx, tx.query(qmemberActor1), p.User, p.Workspace).Scan(&p.Role)
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
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	actor, err = memberActor(ctx, tx, actor)
	if err != nil {
		return err
	}
	var old access.Role
	err = tx.QueryRowContext(ctx, tx.query(qChangeMember1), user, actor.Workspace).Scan(&old)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if (old == access.Owner || old == access.Administrator || role == access.Owner || role == access.Administrator) && !actor.Allows(access.Owners) {
		return fmt.Errorf("owner permission required")
	}
	if old == access.Owner && role != access.Owner {
		var owners int
		if err := tx.QueryRowContext(ctx, tx.query(qChangeMember2), actor.Workspace).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return fmt.Errorf("assign another owner first")
		}
	}
	if role == "" {
		_, err = tx.ExecContext(ctx, tx.query(qChangeMember3), user, actor.Workspace)
	} else {
		var active int
		if err = tx.QueryRowContext(ctx, tx.query(qChangeMember4), user).Scan(&active); err != nil || active != 1 {
			return fmt.Errorf("user unavailable")
		}
		_, err = tx.ExecContext(ctx, tx.query(qChangeMember5), user, actor.Workspace, role)
	}
	if err != nil {
		return err
	}
	if role == "" {
		for _, query := range []string{tx.query(qChangeMember6), tx.query(qChangeMember7), tx.query(qChangeMember8), tx.query(qChangeMember9)} {
			if _, err := tx.ExecContext(ctx, query, user, actor.Workspace); err != nil {
				return err
			}
		}
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
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sole int
	err = tx.QueryRowContext(ctx, tx.query(qDeleteUser1), user).Scan(&sole)
	if err != nil {
		return err
	}
	if sole != 0 {
		return fmt.Errorf("transfer workspace ownership first")
	}
	if _, err = tx.ExecContext(ctx, tx.query(qDeleteUser2), user); err != nil {
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

func audit(ctx context.Context, tx *transaction, actor, workspace, action, target string) error {
	if _, err := tx.ExecContext(ctx, tx.query(qaudit1), actor, workspace, action, target, time.Now().Unix()); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.query(qaudit2))
	return err
}

func (d *DB) Audit(ctx context.Context, workspace string, before int64) ([]Audit, error) {
	rows, err := d.sql.QueryContext(ctx, d.query(qAudit1), workspace, before, before)
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
