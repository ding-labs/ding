package store

import "fmt"

// PurgeWatch removes a deleted watch and its history in the caller's transaction.
// Keep an ID tombstone so delayed inputs or retries cannot target a new watch
// with the same ID and generation. Audit and integration receipts stay intact.
func (t *Tx) PurgeWatch(id string) error {
	r, err := t.Watch(id)
	if err != nil {
		return err
	}
	if _, err := r.LifecycleTarget("purge"); err != nil {
		return err
	}
	if hold, err := t.HandoffHold(id); err != nil {
		return err
	} else if hold != "" {
		return fmt.Errorf("watch has an active transfer")
	}
	for _, query := range []statement{
		{sqlite: `INSERT INTO metadata(key,value) VALUES('purged_watch:'||?, '1')`, postgres: `INSERT INTO metadata(key,value) VALUES('purged_watch:'||$1, '1')`},
		{sqlite: `DELETE FROM outbox WHERE watch_id=?`, postgres: `DELETE FROM outbox WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`},
		{sqlite: `DELETE FROM events WHERE watch_id=?`, postgres: `DELETE FROM events WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`},
		{sqlite: `DELETE FROM observations WHERE watch_id=?`, postgres: `DELETE FROM observations WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`},
		{sqlite: `DELETE FROM entities WHERE watch_id=?`, postgres: `DELETE FROM entities WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`},
		{sqlite: `DELETE FROM timers WHERE watch_id=?`, postgres: `DELETE FROM timers WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`},
		{sqlite: `DELETE FROM input_receipts WHERE watch_id=?`, postgres: `DELETE FROM input_receipts WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`},
		{sqlite: `DELETE FROM handoffs WHERE watch_id=?`, postgres: `DELETE FROM handoffs WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`},
		{sqlite: `DELETE FROM delivery_streams WHERE watch_id=?`, postgres: `DELETE FROM delivery_streams WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`},
		{sqlite: `DELETE FROM watches WHERE id=?`, postgres: `DELETE FROM watches WHERE workspace_id=current_setting('ding.workspace') AND id=$1`},
		{sqlite: `DELETE FROM watch_revisions WHERE watch_id=?`, postgres: `DELETE FROM watch_revisions WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`},
	} {
		if _, err := t.exec(query, id); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tx) checkPurgedWatch(id string) error {
	var purged bool
	if err := t.sql.QueryRowContext(t.ctx, t.query(statement{sqlite: `SELECT EXISTS(SELECT 1 FROM metadata WHERE key='purged_watch:'||?)`, postgres: `SELECT EXISTS(SELECT 1 FROM metadata WHERE workspace_id=current_setting('ding.workspace') AND key='purged_watch:'||$1)`}), id).Scan(&purged); err != nil {
		return err
	}
	if purged {
		return fmt.Errorf("permanently deleted watch IDs cannot be reused; use a new watch ID")
	}
	return nil
}
