package store

import "encoding/json"

type postgresEntityPin struct {
	Entity string `json:"entity_key"`
	Kind   string `json:"kind"`
	Value  string `json:"value"`
}

// The Go projection supplies the complete desired references. Disjoint deletes
// and inserts retain unchanged pins without a read or one round trip per pin.
func (t *Tx) syncPostgresEntityPins(id string, keys []string, pins []postgresEntityPin) error {
	if len(keys) == 0 {
		return nil
	}
	if pins == nil {
		pins = []postgresEntityPin{}
	}
	data, err := json.Marshal(pins)
	if err != nil {
		return err
	}
	query := `WITH wanted AS (SELECT * FROM jsonb_to_recordset($3::jsonb)
 AS p(entity_key text,kind text,value text)),
 removed AS (DELETE FROM entity_pins p
 WHERE p.workspace_id=current_setting('ding.workspace') AND p.watch_id=$1
 AND p.entity_key=ANY($2::text[])
 AND NOT EXISTS (SELECT 1 FROM wanted w WHERE
 (w.entity_key,w.kind,w.value)=(p.entity_key,p.kind,p.value)))
 INSERT INTO entity_pins(watch_id,entity_key,kind,value)
 SELECT $1,entity_key,kind,value FROM wanted
 ON CONFLICT(workspace_id,watch_id,entity_key,kind,value) DO NOTHING`
	_, err = t.execSQL(query, id, keys, string(data))
	return err
}
