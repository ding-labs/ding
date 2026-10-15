package store

import (
	"encoding/json"
	"time"
)

// EntityCount checks group admission without loading state blobs. The caller
// updates its local count as it creates or removes entities in the transaction.
func (t *Tx) EntityCount(id string) (int, error) {
	var n int
	err := t.sql.QueryRowContext(t.ctx, t.query(qEntityCount), id).Scan(&n)
	return n, err
}

func (t *Tx) Entities(id string) (map[string]json.RawMessage, error) {
	rows, err := t.sql.QueryContext(t.ctx, t.query(qEntities), id)
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
	query := t.query(qAllOutbox)
	var args []any
	if id != "" {
		query = t.query(qWatchOutbox)
		args = []any{id}
	}
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
		_, err := t.exec(qLastError1, detail, id, generation)
		return err
	}
	_, err := t.exec(qLastError2, detail, timestamp(next), id, generation)
	return err
}

func (t *Tx) Pending() (int, error) {
	var n int
	err := t.sql.QueryRowContext(t.ctx, t.query(qPending1)).Scan(&n)
	return n, err
}
