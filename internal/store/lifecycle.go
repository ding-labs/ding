package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/watch"
)

func (t *Tx) ResetState(id string) error {
	for _, q := range []statement{qResetEntities, qResetTimers, qResetReceipts} {
		if _, err := t.exec(q, id); err != nil {
			return err
		}
	}
	return nil
}
func (t *Tx) DeleteEntity(id, key string) error {
	_, err := t.exec(qDeleteEntity1, id, key)
	return err
}
func (t *Tx) DeleteTimers(id string) error {
	_, err := t.exec(qDeleteTimers1, id)
	return err
}
func (t *Tx) Timer(id, key, kind string) (Timer, error) {
	timer := Timer{WatchID: id, Entity: key, Kind: kind}
	var due int64
	err := t.sql.QueryRowContext(t.ctx, t.query(qTimer1), id, key, kind).Scan(&timer.Generation, &due, &timer.Body)
	timer.Due = instant(due)
	return timer, missing(err)
}
func (t *Tx) DeleteTimer(timer Timer) error {
	_, err := t.exec(qDeleteTimer1, timer.WatchID, timer.Entity, timer.Kind, timer.Generation, timestamp(timer.Due))
	return err
}
func (t *Tx) DueTimers(now time.Time, limit int) ([]Timer, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("invalid timer limit")
	}
	rows, err := t.sql.QueryContext(t.ctx, t.query(qDueTimers1), timestamp(now), limit)
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
	_, err := t.exec(qCancelDeliveries1, timestamp(now), id)
	if err != nil {
		return err
	}
	_, err = t.exec(qCancelDeliveries2, id)
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
	if _, err := t.exec(qRetain1); err != nil {
		return err
	}
	if _, err := t.exec(qRetain2); err != nil {
		return err
	}
	if _, err := t.exec(qRetain3); err != nil {
		return err
	}
	if _, err := t.exec(qRetain4); err != nil {
		return err
	}
	for _, id := range eventPins {
		if id != "" {
			if _, err := t.exec(qRetain5, id); err != nil {
				return err
			}
		}
	}
	for _, sequence := range pins {
		if sequence < 1 {
			continue
		}
		if _, err := t.exec(qRetain6, sequence); err != nil {
			return err
		}
	}
	for _, q := range []struct {
		sql string
		arg any
	}{
		{t.query(qRetain7), timestamp(now)},
		{t.query(qRetain8), timestamp(now)},
		{t.query(qRetain9), timestamp(now)},
		{t.query(qRetain10), timestamp(cutoff)},
		{t.query(qRetain11), timestamp(cutoff)},
		{t.query(qRetain12), timestamp(cutoff)},
		{t.query(qRetain13), timestamp(cutoff)},
		{t.query(qRetain14), timestamp(cutoff)},
		{t.query(qRetain15), timestamp(cutoff)},
	} {
		if _, err := t.execSQL(q.sql, q.arg); err != nil {
			return err
		}
	}
	return nil
}

// RetainLogSummaries follows Retain in the same transaction, sharing its input
// pins. Event evidence and pending delivery history retain the global policy.
func (t *Tx) RetainLogSummaries(id string, cutoff time.Time) error {
	_, err := t.exec(qRetainLogSummaries1, id, timestamp(cutoff), id)
	return err
}

// RevisionState changes only the metadata pointer of compatible entity state.
func (t *Tx) RevisionState(id, revision string, generation int64) error {
	if _, err := t.exec(qRevisionState1, revision, id); err != nil {
		return err
	}
	_, err := t.exec(qRevisionState2, generation, id)
	return err
}
func (t *Tx) Observations(id string, sequences []int64) ([]json.RawMessage, error) {
	var out []json.RawMessage
	for _, seq := range sequences {
		var body []byte
		err := t.sql.QueryRowContext(t.ctx, t.query(qObservations1), id, seq).Scan(&body)
		if err != nil {
			return nil, missing(err)
		}
		var o watch.Observation
		if err := json.Unmarshal(body, &o); err != nil {
			return nil, err
		}
		o.Sequence = seq
		encoded, err := json.Marshal(o)
		if err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(encoded))
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
