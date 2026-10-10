package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type HandoffProof struct {
	Revision     string            `json:"revision"`
	Destinations map[string]string `json:"destinations"`
	Credentials  map[string]string `json:"credentials"`
	At           time.Time         `json:"at"`
}

func (d *DB) SaveHandoffProof(ctx context.Context, account, id string, p HandoffProof) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(raw) > 32<<10 || len(id) != 64 {
		return fmt.Errorf("invalid handoff proof")
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM handoff_proofs WHERE account=? AND id!=?", account, id).Scan(&n); err != nil {
		return err
	}
	if n >= 1000 {
		return fmt.Errorf("handoff proof capacity reached")
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO handoff_proofs(account,id,body) VALUES(?,?,?) ON CONFLICT(account,id) DO UPDATE SET body=excluded.body", account, id, raw)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (d *DB) HandoffProof(ctx context.Context, account, id string) (HandoffProof, error) {
	var p HandoffProof
	var raw []byte
	err := d.sql.QueryRowContext(ctx, "SELECT body FROM handoff_proofs WHERE account=? AND id=?", account, id).Scan(&raw)
	if err != nil {
		return p, err
	}
	return p, json.Unmarshal(raw, &p)
}
