package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/ding-labs/ding/internal/access"
)

type Audit struct {
	Sequence  int64     `json:"sequence"`
	Actor     string    `json:"actor"`
	Workspace string    `json:"workspace"`
	Grant     string    `json:"grant,omitempty"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	At        time.Time `json:"at"`
}

func auditMutation(ctx context.Context, tx *sql.Tx, p access.Principal) error {
	action, target := access.Operation(ctx)
	if action == "" {
		action = "update"
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit(actor,workspace,grant_id,action,target,at) VALUES(?,?,?,?,?,?)", p.User, p.Workspace, p.Grant, action, target, time.Now().Unix()); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM audit WHERE sequence <= (SELECT COALESCE(MAX(sequence),0)-10000 FROM audit)")
	return err
}
func (t *Tx) Audit(before int64) ([]Audit, error) {
	rows, err := t.sql.QueryContext(t.ctx, "SELECT sequence,actor,workspace,grant_id,action,target,at FROM audit WHERE (?=0 OR sequence<?) ORDER BY sequence DESC LIMIT 100", before, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Audit{}
	for rows.Next() {
		var a Audit
		var at int64
		if err := rows.Scan(&a.Sequence, &a.Actor, &a.Workspace, &a.Grant, &a.Action, &a.Target, &at); err != nil {
			return nil, err
		}
		a.At = time.Unix(at, 0).UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}
