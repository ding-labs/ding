package store

import (
	"encoding/json"
	"strings"
)

// EntitySubset reads only the groups touched by an input batch. Missing keys are
// absent from the result; workspace scope is enforced by SQL and PostgreSQL RLS.
func (t *Tx) EntitySubset(id string, keys []string) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if len(keys) == 0 {
		return out, nil
	}
	args := []any{id}
	query := "SELECT entity_key,state FROM entities WHERE watch_id=? AND entity_key IN (" + strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",") + ")"
	if t.postgres {
		query = "SELECT entity_key,state FROM entities WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND entity_key=ANY($2::text[])"
		args = append(args, keys)
	} else {
		for _, key := range keys {
			args = append(args, key)
		}
	}
	rows, err := t.sql.QueryContext(t.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var data []byte
		if err := rows.Scan(&key, &data); err != nil {
			return nil, err
		}
		out[key] = data
	}
	return out, rows.Err()
}
