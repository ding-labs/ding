package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/watch"
)

var ErrCursor = errors.New("invalid event cursor")
var ErrCursorExpired = errors.New("event cursor expired")

type eventCursor struct {
	Version int    `json:"v"`
	Store   string `json:"store"`
	Watch   string `json:"watch"`
	After   int64  `json:"after"`
}
type EventPage struct {
	Events []watch.Event `json:"events"`
	Cursor string        `json:"cursor"`
	More   bool          `json:"more"`
}

func (t *Tx) EventPage(id, cursor string, limit int) (EventPage, error) {
	page := EventPage{Events: []watch.Event{}}
	if limit < 1 || limit > 1000 {
		return page, ErrCursor
	}
	var identity string
	if err := t.sql.QueryRowContext(t.ctx, t.query(qEventPage1)).Scan(&identity); err != nil {
		return page, err
	}
	var high int64
	if err := t.sql.QueryRowContext(t.ctx, t.query(qEventPage2)).Scan(&high); err != nil {
		return page, err
	}
	c := eventCursor{Version: 1, Store: identity, Watch: id}
	if cursor != "" {
		if len(cursor) > 1024 {
			return page, ErrCursor
		}
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.Store != identity || c.Watch != id || c.After < 0 || c.After > high {
			return page, ErrCursor
		}
		var floor int64
		if err := t.sql.QueryRowContext(t.ctx, t.query(qEventPage3), "event_floor:"+id).Scan(&floor); err != nil {
			return page, err
		}
		if c.After < floor {
			return page, ErrCursorExpired
		}
	}
	events, err := t.Events(id, c.After, limit)
	if err != nil {
		return page, err
	}
	// The response remains comfortably below the client's 16 MiB envelope bound.
	size := 0
	for _, event := range events {
		b, err := json.Marshal(event)
		if err != nil {
			return page, err
		}
		if size+len(b) > 8<<20 {
			break
		}
		size += len(b)
		page.Events = append(page.Events, event)
		c.After = event.Sequence
	}
	page.More = len(events) == limit || len(page.Events) < len(events)
	if !page.More {
		c.After = high
	}
	b, err := json.Marshal(c)
	if err != nil {
		return page, err
	}
	page.Cursor = base64.RawURLEncoding.EncodeToString(b)
	return page, nil
}
func (t *Tx) Event(id string) (watch.Event, error) {
	var e watch.Event
	var raw []byte
	var sequence int64
	if err := t.sql.QueryRowContext(t.ctx, t.query(qEvent1), id).Scan(&sequence, &raw); err != nil {
		return e, missing(err)
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return e, err
	}
	e.Sequence = sequence
	return e, nil
}
func (t *Tx) Definition(id, revision string) (watch.Definition, error) {
	var d watch.Definition
	var raw []byte
	if err := t.sql.QueryRowContext(t.ctx, t.query(qDefinition1), id, revision).Scan(&raw); err != nil {
		return d, missing(err)
	}
	return d, json.Unmarshal(raw, &d)
}
func (t *Tx) SaveReplay(id string, checkpoint any) error {
	raw, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	_, err = t.exec(qSaveReplay1, id, raw)
	return err
}
func (t *Tx) Replay(id string) (json.RawMessage, error) {
	var raw []byte
	err := t.sql.QueryRowContext(t.ctx, t.query(qReplay1), id).Scan(&raw)
	return raw, missing(err)
}

type Attempt struct {
	Attempt int       `json:"attempt"`
	At      time.Time `json:"at"`
	Outcome string    `json:"outcome"`
	Detail  string    `json:"detail"`
}
type DeliveryInspection struct {
	Intent   Intent    `json:"intent"`
	Attempts []Attempt `json:"attempts"`
	Before   int64     `json:"before"`
}

func (t *Tx) Delivery(id, before int64) (DeliveryInspection, error) {
	var out DeliveryInspection
	var err error
	out.Intent, err = t.Intent(id)
	if err != nil {
		return out, err
	}
	out.Attempts = []Attempt{}
	if before == 0 {
		before = 1<<63 - 1
	}
	rows, err := t.sql.QueryContext(t.ctx, t.query(qDelivery1), id, before)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var a Attempt
		var at int64
		if err := rows.Scan(&out.Before, &a.Attempt, &at, &a.Outcome, &a.Detail); err != nil {
			return out, err
		}
		a.At = instant(at)
		out.Attempts = append(out.Attempts, a)
	}
	return out, rows.Err()
}
func (t *Tx) Retry(id int64, now time.Time) error {
	i, err := t.Intent(id)
	if err != nil {
		return err
	}
	if i.Status != "permanent" && i.Status != "exhausted" && i.Status != "canceled" {
		return ErrConflict
	}
	if _, err = t.exec(qRetry1, id, i.Attempts, timestamp(now)); err != nil {
		return err
	}
	_, err = t.exec(qRetry2, timestamp(now), timestamp(now), id)
	return err
}
func (t *Tx) DeliveryCounts() (map[string]int, error) {
	counts := map[string]int{}
	rows, err := t.sql.QueryContext(t.ctx, t.query(qDeliveryCounts1))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		counts[status] = n
	}
	return counts, rows.Err()
}
func (t *Tx) Destinations() ([]watch.Destination, error) {
	rows, err := t.sql.QueryContext(t.ctx, t.query(qDestinations1))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []watch.Destination{}
	for rows.Next() {
		var raw []byte
		var d watch.Destination
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("invalid stored destination")
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type ObservationPage struct {
	Observations []watch.Observation `json:"observations"`
	After        int64               `json:"after"`
	More         bool                `json:"more"`
}

func (t *Tx) EvidenceInputs(event string, after int64) (ObservationPage, error) {
	out := ObservationPage{Observations: []watch.Observation{}, After: after}
	if _, err := t.Event(event); err != nil {
		return out, err
	}
	rows, err := t.sql.QueryContext(t.ctx, t.query(qEvidenceInputs1), event, after)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	size := 0
	for rows.Next() {
		var seq int64
		var raw []byte
		var o watch.Observation
		if err := rows.Scan(&seq, &raw); err != nil {
			return out, err
		}
		if size+len(raw) > 4<<20 {
			out.More = true
			break
		}
		size += len(raw)
		if err := json.Unmarshal(raw, &o); err != nil {
			return out, err
		}
		o.Sequence = seq
		out.After = seq
		out.Observations = append(out.Observations, o)
	}
	if len(out.Observations) == 100 {
		out.More = true
	}
	return out, rows.Err()
}

type EntitySummary struct {
	WaitingReason   string `json:"waitingReason,omitempty"`
	Key             string `json:"key"`
	Open            bool   `json:"open"`
	SourceUnhealthy bool   `json:"sourceUnhealthy"`
	Matches         int    `json:"matches"`
	Recoveries      int    `json:"recoveries"`
	LastSequence    int64  `json:"lastSequence"`
	Samples         int    `json:"samples"`
}
type DeliverySummary struct {
	ID                  int64     `json:"id"`
	EventID             string    `json:"eventId"`
	DestinationID       string    `json:"destinationId"`
	DestinationRevision string    `json:"destinationRevision"`
	Status              string    `json:"status"`
	Attempts            int       `json:"attempts"`
	NextAt              time.Time `json:"nextAt"`
	LastError           string    `json:"lastError,omitempty"`
}
type WatchSummary struct {
	LogObservation   *watch.Observation `json:"logObservation,omitempty"`
	Watch            WatchRecord        `json:"watch"`
	Entities         []EntitySummary    `json:"entities"`
	EntitiesAfter    string             `json:"entitiesAfter"`
	EntitiesMore     bool               `json:"entitiesMore"`
	Deliveries       []DeliverySummary  `json:"deliveries"`
	DeliveriesBefore int64              `json:"deliveriesBefore"`
	DeliveriesMore   bool               `json:"deliveriesMore"`
}

func (t *Tx) InspectSummary(id, after string, before int64) (WatchSummary, error) {
	out := WatchSummary{Entities: []EntitySummary{}, Deliveries: []DeliverySummary{}}
	var err error
	out.Watch, err = t.Watch(id)
	if err != nil {
		return out, err
	}
	if out.Watch.Plan.Definition.Spec.Source.Type == "logs" {
		var body []byte
		var sequence int64
		err := t.sql.QueryRowContext(t.ctx, t.query(qInspectSummary1), id, out.Watch.Generation).Scan(&sequence, &body)
		if err != nil && err != sql.ErrNoRows {
			return out, err
		}
		if err == nil {
			out.LogObservation = &watch.Observation{}
			if err := json.Unmarshal(body, out.LogObservation); err != nil {
				return out, err
			}
			out.LogObservation.Sequence = sequence
		}
	}
	rows, err := t.sql.QueryContext(t.ctx, t.query(qInspectSummary2), id, after)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var e EntitySummary
		if err := rows.Scan(&e.Key, &e.Open, &e.SourceUnhealthy, &e.Matches, &e.Recoveries, &e.LastSequence, &e.Samples, &e.WaitingReason); err != nil {
			rows.Close()
			return out, err
		}
		if len(out.Entities) == 100 {
			out.EntitiesMore = true
			break
		}
		out.Entities = append(out.Entities, e)
		out.EntitiesAfter = e.Key
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if before == 0 {
		before = 1<<63 - 1
	}
	rows, err = t.sql.QueryContext(t.ctx, t.query(qInspectSummary3), id, before)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var d DeliverySummary
		var next int64
		if err := rows.Scan(&d.ID, &d.EventID, &d.DestinationID, &d.DestinationRevision, &d.Status, &d.Attempts, &next, &d.LastError); err != nil {
			return out, err
		}
		if len(out.Deliveries) == 100 {
			out.DeliveriesMore = true
			break
		}
		d.NextAt = instant(next)
		out.DeliveriesBefore = d.ID
		out.Deliveries = append(out.Deliveries, d)
	}
	return out, rows.Err()
}
func (t *Tx) EntityHealth(id string) (unhealthy, open int, err error) {
	err = t.sql.QueryRowContext(t.ctx, t.query(qEntityHealth1), id).Scan(&unhealthy, &open)
	return
}
