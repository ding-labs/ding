package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

func missing(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func timestamp(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}
func instant(ns int64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns).UTC()
}

type WatchRecord struct {
	Plan        plan.Compiled `json:"plan"`
	Generation  int64         `json:"generation"`
	Status      string        `json:"status"`
	NextAt      time.Time     `json:"nextAt"`
	Cursor      string        `json:"cursor,omitempty"`
	LastInputAt time.Time     `json:"lastInputAt,omitempty"`
	LastError   string        `json:"lastError,omitempty"`
}

func (t *Tx) Watch(id string) (WatchRecord, error) {
	var r WatchRecord
	var data []byte
	var next, last int64
	err := t.sql.QueryRowContext(t.ctx, `SELECT r.definition,r.revision,r.fingerprint,w.generation,w.status,w.next_at,w.cursor,w.last_input_at,w.last_error FROM watches w JOIN watch_revisions r ON r.watch_id=w.id AND r.revision=w.revision WHERE w.id=?`, id).Scan(&data, &r.Plan.Revision, &r.Plan.Fingerprint, &r.Generation, &r.Status, &next, &r.Cursor, &last, &r.LastError)
	if err != nil {
		return r, missing(err)
	}
	if err = json.Unmarshal(data, &r.Plan.Definition); err != nil {
		return r, fmt.Errorf("invalid stored definition: %w", err)
	}
	r.NextAt = instant(next)
	r.LastInputAt = instant(last)
	return r, nil
}
func (t *Tx) Watches() ([]WatchRecord, error) {
	rows, err := t.sql.QueryContext(t.ctx, "SELECT id FROM watches ORDER BY id")
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
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
	result := make([]WatchRecord, 0, len(ids))
	for _, id := range ids {
		r, err := t.Watch(id)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

// SaveWatch persists the immutable definition and active pointer in this
// transaction. Lifecycle compatibility/reset decisions belong to the runtime.
func (t *Tx) SaveWatch(r WatchRecord, now time.Time) error {
	c, err := plan.Compile(r.Plan.Definition)
	if err != nil {
		return err
	}
	if c.Revision != r.Plan.Revision || c.Fingerprint != r.Plan.Fingerprint || r.Generation < 1 {
		return fmt.Errorf("invalid compiled watch metadata")
	}
	data, err := json.Marshal(c.Definition)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, `INSERT INTO watch_revisions(watch_id,revision,fingerprint,definition,created_at) VALUES(?,?,?,?,?) ON CONFLICT(watch_id,revision) DO NOTHING`, c.Definition.Metadata.ID, c.Revision, c.Fingerprint, data, timestamp(now))
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, `INSERT INTO watches(id,revision,generation,status,next_at,cursor,last_input_at,last_error) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,generation=excluded.generation,status=excluded.status,next_at=excluded.next_at,cursor=excluded.cursor,last_input_at=excluded.last_input_at,last_error=excluded.last_error`, c.Definition.Metadata.ID, c.Revision, r.Generation, r.Status, timestamp(r.NextAt), r.Cursor, timestamp(r.LastInputAt), r.LastError)
	return err
}

// Checkpoint fences results acquired for old definitions or a paused watch.
func (t *Tx) Checkpoint(id string, generation int64, nextAt, lastAt time.Time, cursor string) error {
	result, err := t.sql.ExecContext(t.ctx, `UPDATE watches SET next_at=?,last_input_at=?,cursor=?,last_error='' WHERE id=? AND generation=? AND status='running'`, timestamp(nextAt), timestamp(lastAt), cursor, id, generation)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrStale
	}
	return nil
}
func (t *Tx) SaveDestination(c plan.CompiledDestination, now time.Time) error {
	validated, err := plan.CompileDestination(c.Definition)
	if err != nil {
		return err
	}
	if validated.Revision != c.Revision {
		return fmt.Errorf("invalid destination revision")
	}
	data, err := json.Marshal(validated.Definition)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, `INSERT INTO destination_revisions(destination_id,revision,definition,created_at) VALUES(?,?,?,?) ON CONFLICT(destination_id,revision) DO NOTHING`, c.Definition.Metadata.ID, c.Revision, data, timestamp(now))
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, `INSERT INTO destinations(id,revision) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision`, c.Definition.Metadata.ID, c.Revision)
	return err
}
func (t *Tx) Destination(id, revision string) (plan.CompiledDestination, error) {
	var c plan.CompiledDestination
	var data []byte
	query := `SELECT revision,definition FROM destination_revisions WHERE destination_id=? AND revision=?`
	args := []any{id, revision}
	if revision == "" {
		query = `SELECT r.revision,r.definition FROM destinations d JOIN destination_revisions r ON r.destination_id=d.id AND r.revision=d.revision WHERE d.id=?`
		args = []any{id}
	}
	if err := t.sql.QueryRowContext(t.ctx, query, args...).Scan(&c.Revision, &data); err != nil {
		return c, missing(err)
	}
	return c, json.Unmarshal(data, &c.Definition)
}
func (t *Tx) AppendObservation(id, revision string, generation int64, o watch.Observation) (int64, error) {
	data, err := json.Marshal(o)
	if err != nil {
		return 0, err
	}
	result, err := t.sql.ExecContext(t.ctx, `INSERT INTO observations(watch_id,revision,generation,input_id,accepted_at,body) VALUES(?,?,?,?,?,?)`, id, revision, generation, o.InputID, timestamp(o.AcceptedAt), data)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (t *Tx) Entity(id, key string) (json.RawMessage, error) {
	var data []byte
	err := t.sql.QueryRowContext(t.ctx, `SELECT state FROM entities WHERE watch_id=? AND entity_key=?`, id, key).Scan(&data)
	return data, missing(err)
}
func (t *Tx) SaveEntity(id, key, revision string, state any, now time.Time) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, `INSERT INTO entities(watch_id,entity_key,revision,state,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(watch_id,entity_key) DO UPDATE SET revision=excluded.revision,state=excluded.state,updated_at=excluded.updated_at`, id, key, revision, data, timestamp(now))
	return err
}
func (t *Tx) AppendEvent(e watch.Event) (int64, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return 0, err
	}
	result, err := t.sql.ExecContext(t.ctx, `INSERT INTO events(id,watch_id,revision,at,type,body) VALUES(?,?,?,?,?,?)`, e.ID, e.WatchID, e.Revision, timestamp(e.At), e.Type, data)
	if err != nil {
		return 0, err
	}
	seq, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, evidence := range e.Evidence {
		if _, err = t.sql.ExecContext(t.ctx, `INSERT INTO event_evidence(event_id,observation_sequence) VALUES(?,?)`, e.ID, evidence); err != nil {
			return 0, err
		}
	}
	return seq, nil
}
func (t *Tx) Events(id string, after int64, limit int) ([]watch.Event, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("event limit must be 1..1000")
	}
	query := `SELECT sequence,body FROM events WHERE sequence>?`
	args := []any{after}
	if id != "" {
		query += " AND watch_id=?"
		args = append(args, id)
	}
	query += " ORDER BY sequence LIMIT ?"
	args = append(args, limit)
	rows, err := t.sql.QueryContext(t.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []watch.Event{}
	for rows.Next() {
		var e watch.Event
		var data []byte
		var seq int64
		if err = rows.Scan(&seq, &data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		e.Sequence = seq
		result = append(result, e)
	}
	return result, rows.Err()
}
func (t *Tx) Receipt(id string, generation int64, inputID string, now time.Time) (first, last int64, err error) {
	err = t.sql.QueryRowContext(t.ctx, `SELECT first_sequence,last_sequence FROM input_receipts WHERE watch_id=? AND generation=? AND input_id=? AND expires_at>?`, id, generation, inputID, timestamp(now)).Scan(&first, &last)
	return first, last, missing(err)
}
func (t *Tx) SaveReceipt(id string, generation int64, inputID string, first, last int64, expires time.Time) error {
	_, err := t.sql.ExecContext(t.ctx, `INSERT INTO input_receipts(watch_id,generation,input_id,first_sequence,last_sequence,expires_at) VALUES(?,?,?,?,?,?) ON CONFLICT(watch_id,generation,input_id) DO UPDATE SET first_sequence=excluded.first_sequence,last_sequence=excluded.last_sequence,expires_at=excluded.expires_at`, id, generation, inputID, first, last, timestamp(expires))
	return err
}

type Timer struct {
	WatchID, Entity, Kind string
	Generation            int64
	Due                   time.Time
	Body                  json.RawMessage
}

func (t *Tx) SaveTimer(timer Timer) error {
	if len(timer.Body) == 0 {
		timer.Body = json.RawMessage(`{}`)
	}
	if !json.Valid(timer.Body) {
		return fmt.Errorf("invalid timer body")
	}
	_, err := t.sql.ExecContext(t.ctx, `INSERT INTO timers(watch_id,entity_key,kind,generation,due_at,body) VALUES(?,?,?,?,?,?) ON CONFLICT(watch_id,entity_key,kind) DO UPDATE SET generation=excluded.generation,due_at=excluded.due_at,body=excluded.body`, timer.WatchID, timer.Entity, timer.Kind, timer.Generation, timestamp(timer.Due), []byte(timer.Body))
	return err
}
