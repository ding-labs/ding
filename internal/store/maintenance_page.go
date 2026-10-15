package store

import (
	"encoding/json"
	"fmt"
)

type MaintenanceEntity struct {
	WatchID, Key string
	State        json.RawMessage
}
type maintenanceCursor struct{ Watch, Key string }

// NextMaintenanceEntities advances a durable key cursor in this transaction.
// The caller's state changes and the cursor commit or roll back together.
func (t *Tx) NextMaintenanceEntities(limit int) ([]MaintenanceEntity, bool, error) {
	if limit < 1 || limit > 256 {
		return nil, false, fmt.Errorf("invalid entity maintenance page limit")
	}
	var cursor maintenanceCursor
	var value string
	err := t.sql.QueryRowContext(t.ctx, t.query(qMaintenanceCursor)).Scan(&value)
	if missing(err) != ErrNotFound && err != nil {
		return nil, false, err
	}
	if value != "" {
		if err = json.Unmarshal([]byte(value), &cursor); err != nil {
			return nil, false, fmt.Errorf("invalid maintenance cursor")
		}
	}
	rows, err := t.sql.QueryContext(t.ctx, t.query(qMaintenancePage), cursor.Watch, cursor.Key, limit)
	if err != nil {
		return nil, false, err
	}
	out := []MaintenanceEntity{}
	for rows.Next() {
		var e MaintenanceEntity
		if err = rows.Scan(&e.WatchID, &e.Key, &e.State); err != nil {
			rows.Close()
			return nil, false, err
		}
		out = append(out, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	next := ""
	if len(out) > 0 {
		last := out[len(out)-1]
		raw, _ := json.Marshal(maintenanceCursor{last.WatchID, last.Key})
		next = string(raw)
	}
	if next != value {
		if _, err = t.exec(qSaveMaintenanceCursor, next); err != nil {
			return nil, false, err
		}
	}
	return out, len(out) == 0, nil
}

var qMaintenanceCursor = statement{sqlite: `SELECT value FROM metadata WHERE key='maintenance_entity_cursor'`, postgres: `SELECT value FROM metadata WHERE workspace_id=current_setting('ding.workspace') AND key='maintenance_entity_cursor'`}
var qSaveMaintenanceCursor = statement{sqlite: `INSERT INTO metadata(key,value) VALUES('maintenance_entity_cursor',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, postgres: `INSERT INTO metadata(key,value) VALUES('maintenance_entity_cursor',$1) ON CONFLICT(workspace_id,key) DO UPDATE SET value=excluded.value`}
var qMaintenancePage = statement{sqlite: `SELECT watch_id,entity_key,state FROM entities WHERE (watch_id,entity_key)>(?,?) ORDER BY watch_id,entity_key LIMIT ?`, postgres: `SELECT watch_id,entity_key,state FROM entities WHERE workspace_id=current_setting('ding.workspace') AND (watch_id,entity_key)>($1,$2) ORDER BY watch_id,entity_key LIMIT $3`}
