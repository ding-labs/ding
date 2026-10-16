package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SaveEntities persists final encoded states inside the caller's transaction.
// PostgreSQL uses bounded set operations; SQLite retains its embedded writes.
// Callers must flush before reading these states or checking the byte quota.
func (t *Tx) SaveEntities(id, revision string, states map[string]json.RawMessage, now time.Time) error {
	keys := make([]string, 0, len(states))
	for key, data := range states {
		if !json.Valid(data) {
			return fmt.Errorf("invalid entity state")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if !t.postgres || len(keys) == 1 {
		for _, key := range keys {
			if err := t.saveEntityData(id, key, revision, states[key], now); err != nil {
				return err
			}
		}
		return nil
	}
	for len(keys) > 0 {
		n := min(len(keys), 100)
		if err := t.savePostgresEntities(id, revision, keys[:n], states, now); err != nil {
			return err
		}
		keys = keys[n:]
	}
	return nil
}

func (t *Tx) savePostgresEntities(id, revision string, keys []string, states map[string]json.RawMessage, now time.Time) error {
	args := []any{id, revision, timestamp(now)}
	values := make([]string, len(keys))
	var pins []postgresEntityPin
	for i, key := range keys {
		refs, err := entityReferences(states[key])
		if err != nil {
			return err
		}
		for pin := range refs {
			pins = append(pins, postgresEntityPin{key, pin.kind, pin.value})
		}
		values[i] = fmt.Sprintf("($1,$%d::text,$2,$%d::bytea,$3)", len(args)+1, len(args)+2)
		args = append(args, key, []byte(states[key]))
	}
	query := `INSERT INTO entities(watch_id,entity_key,revision,state,updated_at) VALUES ` + strings.Join(values, ",") + `
 ON CONFLICT(workspace_id,watch_id,entity_key) DO UPDATE
 SET revision=excluded.revision,state=excluded.state,updated_at=excluded.updated_at`
	if _, err := t.execSQL(query, args...); err != nil {
		return err
	}
	return t.syncPostgresEntityPins(id, keys, pins)
}
