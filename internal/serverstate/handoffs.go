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

func (d *DB) SaveHandoffProof(ctx context.Context, workspace, id string, p HandoffProof) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(raw) > 32<<10 || len(id) != 64 {
		return fmt.Errorf("invalid handoff proof")
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, tx.query(qSaveHandoffProof1), workspace, id).Scan(&n); err != nil {
		return err
	}
	if n >= 1000 {
		return fmt.Errorf("handoff proof capacity reached")
	}
	_, err = tx.ExecContext(ctx, tx.query(qSaveHandoffProof2), workspace, id, raw)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (d *DB) HandoffProof(ctx context.Context, workspace, id string) (HandoffProof, error) {
	var p HandoffProof
	var raw []byte
	err := d.sql.QueryRowContext(ctx, d.query(qHandoffProof1), workspace, id).Scan(&raw)
	if err != nil {
		return p, err
	}
	return p, json.Unmarshal(raw, &p)
}
