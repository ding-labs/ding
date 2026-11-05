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
	if r.Status != "deleted" {
		return fmt.Errorf("only deleted watches can be permanently deleted")
	}
	if hold, err := t.HandoffHold(id); err != nil {
		return err
	} else if hold != "" {
		return fmt.Errorf("watch has an active transfer")
	}
	for _, query := range []string{
		"INSERT INTO metadata(key,value) VALUES('purged_watch:'||?, '1')",
		"DELETE FROM outbox WHERE watch_id=?",
		"DELETE FROM events WHERE watch_id=?",
		"DELETE FROM observations WHERE watch_id=?",
		"DELETE FROM entities WHERE watch_id=?",
		"DELETE FROM timers WHERE watch_id=?",
		"DELETE FROM input_receipts WHERE watch_id=?",
		"DELETE FROM handoffs WHERE watch_id=?",
		"DELETE FROM watches WHERE id=?",
		"DELETE FROM watch_revisions WHERE watch_id=?",
	} {
		if _, err := t.sql.ExecContext(t.ctx, query, id); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tx) checkPurgedWatch(id string) error {
	var purged bool
	if err := t.sql.QueryRowContext(t.ctx, "SELECT EXISTS(SELECT 1 FROM metadata WHERE key='purged_watch:'||?)", id).Scan(&purged); err != nil {
		return err
	}
	if purged {
		return fmt.Errorf("permanently deleted watch IDs cannot be reused; use a new watch ID")
	}
	return nil
}
