package store

import (
	"encoding/json"
	"time"
)

func (t *Tx) Entities(id string) (map[string]json.RawMessage, error) {
	rows, err := t.sql.QueryContext(t.ctx, "SELECT entity_key,state FROM entities WHERE watch_id=? ORDER BY entity_key", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]json.RawMessage{}
	for rows.Next() {
		var key string
		var data []byte
		if err := rows.Scan(&key, &data); err != nil {
			return nil, err
		}
		result[key] = json.RawMessage(data)
	}
	return result, rows.Err()
}
func (t *Tx) Outbox(id string) ([]Intent, error) {
	query := "SELECT id FROM outbox"
	var args []any
	if id != "" {
		query += " WHERE watch_id=?"
		args = append(args, id)
	}
	query += " ORDER BY id DESC LIMIT 1000"
	rows, err := t.sql.QueryContext(t.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []Intent{}
	for _, id := range ids {
		i, err := t.Intent(id)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}
func (t *Tx) LastError(id string, generation int64, detail string, next time.Time) error {
	if next.IsZero() {
		_, err := t.sql.ExecContext(t.ctx, "UPDATE watches SET last_error=? WHERE id=? AND generation=?", detail, id, generation)
		return err
	}
	_, err := t.sql.ExecContext(t.ctx, "UPDATE watches SET last_error=?,next_at=? WHERE id=? AND generation=?", detail, timestamp(next), id, generation)
	return err
}

func (t *Tx) Pending() (int, error) {
	var n int
	err := t.sql.QueryRowContext(t.ctx, "SELECT count(*) FROM outbox WHERE status IN ('pending','leased')").Scan(&n)
	return n, err
}
