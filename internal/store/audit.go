package store

import (
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

func (t *Tx) auditMutation(p access.Principal) error {
	action, target := access.Operation(t.ctx)
	if action == "" {
		action = "update"
	}
	if t.postgres {
		_, err := t.exec(qAuditMutationRetained, p.User, p.Workspace, p.Grant, action, target, time.Now().Unix())
		return err
	}
	if _, err := t.exec(qauditMutation1, p.User, p.Workspace, p.Grant, action, target, time.Now().Unix()); err != nil {
		return err
	}
	_, err := t.exec(qauditMutation2)
	return err
}
func (t *Tx) Audit(before int64) ([]Audit, error) {
	rows, err := t.sql.QueryContext(t.ctx, t.query(qAudit1), before, before)
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
