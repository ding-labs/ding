package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (t *Tx) ResetState(id string) error {
	for _, table := range []string{"entities", "timers", "input_receipts"} {
		if _, err := t.sql.ExecContext(t.ctx, "DELETE FROM "+table+" WHERE watch_id=?", id); err != nil {
			return err
		}
	}
	return nil
}
func (t *Tx) DeleteEntity(id, key string) error {
	_, err := t.sql.ExecContext(t.ctx, "DELETE FROM entities WHERE watch_id=? AND entity_key=?", id, key)
	return err
}
func (t *Tx) DeleteTimers(id string) error {
	_, err := t.sql.ExecContext(t.ctx, "DELETE FROM timers WHERE watch_id=?", id)
	return err
}
func (t *Tx) Timer(id, key, kind string) (Timer, error) {
	timer := Timer{WatchID: id, Entity: key, Kind: kind}
	var due int64
	err := t.sql.QueryRowContext(t.ctx, "SELECT generation,due_at,body FROM timers WHERE watch_id=? AND entity_key=? AND kind=?", id, key, kind).Scan(&timer.Generation, &due, &timer.Body)
	timer.Due = instant(due)
	return timer, missing(err)
}
func (t *Tx) DeleteTimer(timer Timer) error {
	_, err := t.sql.ExecContext(t.ctx, "DELETE FROM timers WHERE watch_id=? AND entity_key=? AND kind=? AND generation=? AND due_at=?", timer.WatchID, timer.Entity, timer.Kind, timer.Generation, timestamp(timer.Due))
	return err
}
func (t *Tx) DueTimers(now time.Time, limit int) ([]Timer, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("invalid timer limit")
	}
	rows, err := t.sql.QueryContext(t.ctx, `SELECT t.watch_id,t.entity_key,t.kind,t.generation,t.due_at,t.body FROM timers t JOIN watches w ON w.id=t.watch_id WHERE w.status='running' AND t.generation=w.generation AND t.due_at<=? ORDER BY t.due_at,t.watch_id,t.entity_key LIMIT ?`, timestamp(now), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Timer{}
	for rows.Next() {
		var timer Timer
		var due int64
		if err := rows.Scan(&timer.WatchID, &timer.Entity, &timer.Kind, &timer.Generation, &due, &timer.Body); err != nil {
			return nil, err
		}
		timer.Due = instant(due)
		out = append(out, timer)
	}
	return out, rows.Err()
}
func (t *Tx) CancelDeliveries(id string, now time.Time) error {
	// This invalidates active leases. A request already accepted remotely cannot
	// be unsent; its late acknowledgment will be rejected as stale.
	_, err := t.sql.ExecContext(t.ctx, `INSERT INTO delivery_attempts(outbox_id,attempt,at,outcome,detail) SELECT id,attempts,?,'canceled','explicit cancellation' FROM outbox WHERE watch_id=? AND status IN ('pending','leased')`, timestamp(now), id)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, `UPDATE outbox SET status='canceled',lease_token='',lease_until=0,last_error='explicit cancellation' WHERE watch_id=? AND status IN ('pending','leased')`, id)
	return err
}

type Usage struct {
	Watches      int   `json:"watches"`
	Pending      int   `json:"pending"`
	Bytes        int64 `json:"liveBytes"`
	Observations int   `json:"observations"`
	Events       int   `json:"events"`
	Entities     int   `json:"entities"`
	Timers       int   `json:"timers"`
}

func (t *Tx) Usage() (Usage, error) {
	var u Usage
	for _, q := range []struct {
		sql  string
		dest any
	}{{"SELECT count(*) FROM watches WHERE status!='deleted'", &u.Watches}, {"SELECT count(*) FROM outbox WHERE status IN ('pending','leased')", &u.Pending}, {"SELECT count(*) FROM observations", &u.Observations}, {"SELECT count(*) FROM events", &u.Events}, {"SELECT count(*) FROM entities", &u.Entities}, {"SELECT count(*) FROM timers", &u.Timers}} {
		if err := t.sql.QueryRowContext(t.ctx, q.sql).Scan(q.dest); err != nil {
			return u, err
		}
	}
	var pages, free, size int64
	for _, q := range []struct {
		sql  string
		dest *int64
	}{{"PRAGMA page_count", &pages}, {"PRAGMA freelist_count", &free}, {"PRAGMA page_size", &size}} {
		if err := t.sql.QueryRowContext(t.ctx, q.sql).Scan(q.dest); err != nil {
			return u, err
		}
	}
	u.Bytes = (pages - free) * size
	return u, nil
}

// Retain removes old history only after its durable references are no longer
// needed. The caller supplies sequence pins from all active entity state.
func (t *Tx) Retain(cutoff, now time.Time, pins []int64, eventPins []string) error {
	if _, err := t.sql.ExecContext(t.ctx, "CREATE TEMP TABLE IF NOT EXISTS retained_inputs(sequence INTEGER PRIMARY KEY)"); err != nil {
		return err
	}
	if _, err := t.sql.ExecContext(t.ctx, "DELETE FROM retained_inputs"); err != nil {
		return err
	}
	if _, err := t.sql.ExecContext(t.ctx, "CREATE TEMP TABLE IF NOT EXISTS retained_events(id TEXT PRIMARY KEY)"); err != nil {
		return err
	}
	if _, err := t.sql.ExecContext(t.ctx, "DELETE FROM retained_events"); err != nil {
		return err
	}
	for _, id := range eventPins {
		if id != "" {
			if _, err := t.sql.ExecContext(t.ctx, "INSERT OR IGNORE INTO retained_events(id) VALUES(?)", id); err != nil {
				return err
			}
		}
	}
	for _, sequence := range pins {
		if sequence < 1 {
			continue
		}
		if _, err := t.sql.ExecContext(t.ctx, "INSERT OR IGNORE INTO retained_inputs(sequence) VALUES(?)", sequence); err != nil {
			return err
		}
	}
	for _, q := range []struct {
		sql string
		arg any
	}{
		{"DELETE FROM input_receipts WHERE expires_at<=?", timestamp(now)},
		{"DELETE FROM outbox WHERE created_at<? AND status NOT IN ('pending','leased')", timestamp(cutoff)},
		{"DELETE FROM events WHERE at<? AND NOT EXISTS (SELECT 1 FROM outbox WHERE event_id=events.id) AND NOT EXISTS (SELECT 1 FROM retained_events WHERE retained_events.id=events.id)", timestamp(cutoff)},
		{"DELETE FROM observations WHERE accepted_at<? AND NOT EXISTS (SELECT 1 FROM event_evidence WHERE observation_sequence=observations.sequence) AND NOT EXISTS (SELECT 1 FROM retained_inputs WHERE sequence=observations.sequence)", timestamp(cutoff)},
		{`DELETE FROM watches WHERE status='deleted' AND next_at<? AND NOT EXISTS (SELECT 1 FROM entities WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM observations WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM events WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM outbox WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM timers WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM input_receipts WHERE watch_id=watches.id)`, timestamp(cutoff)},
		{`DELETE FROM watch_revisions WHERE created_at<? AND NOT EXISTS (SELECT 1 FROM watches WHERE id=watch_id AND watches.revision=watch_revisions.revision) AND NOT EXISTS (SELECT 1 FROM entities WHERE entities.watch_id=watch_revisions.watch_id AND entities.revision=watch_revisions.revision) AND NOT EXISTS (SELECT 1 FROM observations WHERE observations.watch_id=watch_revisions.watch_id AND observations.revision=watch_revisions.revision) AND NOT EXISTS (SELECT 1 FROM events WHERE events.watch_id=watch_revisions.watch_id AND events.revision=watch_revisions.revision)`, timestamp(cutoff)},
		{`DELETE FROM destination_revisions WHERE created_at<? AND NOT EXISTS (SELECT 1 FROM destinations WHERE destinations.id=destination_id AND destinations.revision=destination_revisions.revision) AND NOT EXISTS (SELECT 1 FROM outbox WHERE outbox.destination_id=destination_revisions.destination_id AND outbox.destination_revision=destination_revisions.revision)`, timestamp(cutoff)},
	} {
		if _, err := t.sql.ExecContext(t.ctx, q.sql, q.arg); err != nil {
			return err
		}
	}
	return nil
}

// RevisionState changes only the metadata pointer of compatible entity state.
func (t *Tx) RevisionState(id, revision string, generation int64) error {
	if _, err := t.sql.ExecContext(t.ctx, "UPDATE entities SET revision=? WHERE watch_id=?", revision, id); err != nil {
		return err
	}
	_, err := t.sql.ExecContext(t.ctx, "UPDATE timers SET generation=? WHERE watch_id=?", generation, id)
	return err
}
func (t *Tx) Observations(id string, sequences []int64) ([]json.RawMessage, error) {
	var out []json.RawMessage
	for _, seq := range sequences {
		var body []byte
		err := t.sql.QueryRowContext(t.ctx, "SELECT body FROM observations WHERE watch_id=? AND sequence=?", id, seq).Scan(&body)
		if err != nil {
			return nil, missing(err)
		}
		out = append(out, json.RawMessage(body))
	}
	return out, nil
}

func (t *Tx) Budget() (Usage, error) {
	var u Usage
	var pages, free, size int64
	for _, q := range []struct {
		sql  string
		dest any
	}{{"SELECT count(*) FROM watches WHERE status!='deleted'", &u.Watches}, {"SELECT count(*) FROM outbox WHERE status IN ('pending','leased')", &u.Pending}, {"PRAGMA page_count", &pages}, {"PRAGMA freelist_count", &free}, {"PRAGMA page_size", &size}} {
		if err := t.sql.QueryRowContext(t.ctx, q.sql).Scan(q.dest); err != nil {
			return u, err
		}
	}
	u.Bytes = (pages - free) * size
	return u, nil
}

// LimitBytes is a hard SQLite page allocation cap. WAL and backup files are
// additional filesystem usage and remain separately visible in Health.
func (s *Store) LimitBytes(ctx context.Context, max int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	var size int64
	if err := s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&size); err != nil {
		return err
	}
	pages := max / size
	if pages < 1 {
		return fmt.Errorf("store byte limit is too small")
	}
	var actual int64
	if err := s.db.QueryRowContext(ctx, fmt.Sprintf("PRAGMA max_page_count=%d", pages)).Scan(&actual); err != nil {
		return err
	}
	if actual > pages {
		return fmt.Errorf("existing database exceeds requested page limit; compact or restore it offline")
	}
	return nil
}
