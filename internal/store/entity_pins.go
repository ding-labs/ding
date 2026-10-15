package store

import (
	"encoding/json"
	"strconv"
)

type entityPin struct{ kind, value string }

// This is a persistence projection of references in the state wire format.
// Condition evaluation and reference lifetimes remain in the shared Go runtime.
func entityReferences(data []byte) (map[entityPin]bool, error) {
	pins := map[entityPin]bool{}
	if len(data) == 0 || data[0] != '{' {
		return pins, nil
	}
	var refs struct {
		LastSequence, FreshSequence, BaselineSequence int64
		Evidence                                      []int64
		Samples                                       []struct{ Sequence int64 }
		IncidentEvent, HealthEvent                    string
	}
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil, err
	}
	for _, seq := range append(refs.Evidence, refs.LastSequence, refs.FreshSequence, refs.BaselineSequence) {
		if seq > 0 {
			pins[entityPin{"input", strconv.FormatInt(seq, 10)}] = true
		}
	}
	for _, sample := range refs.Samples {
		if sample.Sequence > 0 {
			pins[entityPin{"input", strconv.FormatInt(sample.Sequence, 10)}] = true
		}
	}
	for _, event := range []string{refs.IncidentEvent, refs.HealthEvent} {
		if event != "" {
			pins[entityPin{"event", event}] = true
		}
	}
	return pins, nil
}

func (t *Tx) syncEntityPins(id, key string, data []byte) error {
	wanted, err := entityReferences(data)
	if err != nil {
		return err
	}
	rows, err := t.sql.QueryContext(t.ctx, t.query(qEntityPins), id, key)
	if err != nil {
		return err
	}
	var remove []entityPin
	for rows.Next() {
		var pin entityPin
		if err = rows.Scan(&pin.kind, &pin.value); err != nil {
			rows.Close()
			return err
		}
		if wanted[pin] {
			delete(wanted, pin)
		} else {
			remove = append(remove, pin)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, pin := range remove {
		if _, err = t.exec(qDeleteEntityPin, id, key, pin.kind, pin.value); err != nil {
			return err
		}
	}
	for pin := range wanted {
		if _, err = t.exec(qInsertEntityPin, id, key, pin.kind, pin.value); err != nil {
			return err
		}
	}
	return nil
}

var qEntityPins = statement{sqlite: `SELECT kind,value FROM entity_pins WHERE watch_id=? AND entity_key=?`, postgres: `SELECT kind,value FROM entity_pins WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND entity_key=$2`}
var qDeleteEntityPin = statement{sqlite: `DELETE FROM entity_pins WHERE watch_id=? AND entity_key=? AND kind=? AND value=?`, postgres: `DELETE FROM entity_pins WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND entity_key=$2 AND kind=$3 AND value=$4`}
var qInsertEntityPin = statement{sqlite: `INSERT INTO entity_pins(watch_id,entity_key,kind,value) VALUES(?,?,?,?)`, postgres: `INSERT INTO entity_pins(watch_id,entity_key,kind,value) VALUES($1,$2,$3,$4)`}
