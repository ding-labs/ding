package store

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/watch"
)

// ConsoleQuery describes a bounded operational view. Cursor identity includes every filter.
// Watch state is a live snapshot; event pages have a fixed upper sequence boundary.
type ConsoleQuery struct {
	Search      string `json:"search"`
	Status      string `json:"status"`
	Source      string `json:"source"`
	Attention   string `json:"attention"`
	Incident    string `json:"incident"`
	Health      string `json:"health"`
	Delivery    string `json:"delivery"`
	Watch       string `json:"watch"`
	Event       string `json:"event"`
	Destination string `json:"destination"`
	Type        string `json:"type"`
	From        string `json:"from"`
	To          string `json:"to"`
	Cursor      string `json:"-"`
	Limit       int    `json:"limit"`
}
type ConsoleWatch struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Revision    string    `json:"revision"`
	Status      string    `json:"status"`
	Source      string    `json:"source"`
	Trigger     string    `json:"trigger"`
	Operator    string    `json:"operator"`
	Generation  int64     `json:"generation"`
	NextAt      time.Time `json:"nextAt"`
	LastInputAt time.Time `json:"lastInputAt"`
	LastError   string    `json:"lastError"`
	Entities    int       `json:"entities"`
	Open        int       `json:"open"`
	Unhealthy   int       `json:"unhealthy"`
	Failed      int       `json:"failed"`
	Pending     int       `json:"pending"`
	Missing     []string  `json:"missing"`
}
type WatchPage struct {
	Watches   []ConsoleWatch `json:"watches"`
	Total     int            `json:"total"`
	All       int            `json:"all"`
	Attention int            `json:"attention"`
	Paused    int            `json:"paused"`
	Cursor    string         `json:"cursor"`
	More      bool           `json:"more"`
	At        time.Time      `json:"at"`
}
type EventSummary struct {
	ID       string    `json:"id"`
	Sequence int64     `json:"sequence"`
	WatchID  string    `json:"watchId"`
	Revision string    `json:"revision"`
	Type     string    `json:"type"`
	At       time.Time `json:"at"`
	Message  string    `json:"message"`
	Entity   string    `json:"entity"`
}
type EventSummaryPage struct {
	Events       []EventSummary `json:"events"`
	Total        int            `json:"total"`
	Cursor       string         `json:"cursor"`
	FollowCursor string         `json:"followCursor"`
	More         bool           `json:"more"`
	At           time.Time      `json:"at"`
}
type ConsoleDelivery struct {
	ID                  int64     `json:"id"`
	EventID             string    `json:"eventId"`
	WatchID             string    `json:"watchId"`
	DestinationID       string    `json:"destinationId"`
	DestinationRevision string    `json:"destinationRevision"`
	Status              string    `json:"status"`
	Attempts            int       `json:"attempts"`
	NextAt              time.Time `json:"nextAt"`
	CreatedAt           time.Time `json:"createdAt"`
	LastError           string    `json:"lastError"`
}
type DeliveryPage struct {
	Deliveries []ConsoleDelivery `json:"deliveries"`
	Total      int               `json:"total"`
	Counts     map[string]int    `json:"counts"`
	Cursor     string            `json:"cursor"`
	More       bool              `json:"more"`
	At         time.Time         `json:"at"`
}
type ConsoleDestination struct {
	Definition watch.Destination `json:"definition"`
	Revision   string            `json:"revision"`
	Watches    int               `json:"watches"`
}
type DestinationPage struct {
	Destinations []ConsoleDestination `json:"destinations"`
	Cursor       string               `json:"cursor"`
	More         bool                 `json:"more"`
}
type viewCursor struct {
	Version                 int
	Store, Kind, Query, Key string
	Before, High, Floor     int64
}

func (t *Tx) Identity() (string, error) {
	var id string
	err := t.sql.QueryRowContext(t.ctx, "SELECT value FROM metadata WHERE key='store_id'").Scan(&id)
	return id, err
}
func (t *Tx) viewCursor(kind string, q ConsoleQuery) (viewCursor, error) {
	c := viewCursor{Version: 1, Kind: kind, Before: 1<<63 - 1, High: 1<<63 - 1}
	if q.Limit < 1 || q.Limit > 100 || len(q.Cursor) > 2048 || len(q.Search) > 200 {
		return c, ErrCursor
	}
	raw, _ := json.Marshal(q)
	c.Query = fmt.Sprintf("%x", sha256.Sum256(raw))
	id, err := t.Identity()
	if err != nil {
		return c, err
	}
	c.Store = id
	if kind == "events" {
		if err = t.sql.QueryRowContext(t.ctx, "SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name='events'),0)").Scan(&c.High); err != nil {
			return c, err
		}
		if err = t.sql.QueryRowContext(t.ctx, "SELECT COALESCE((SELECT CAST(value AS INTEGER) FROM metadata WHERE key=?),0)", "event_floor:"+q.Watch).Scan(&c.Floor); err != nil {
			return c, err
		}
	}
	if q.Cursor != "" {
		var old viewCursor
		b, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		if e != nil || json.Unmarshal(b, &old) != nil || old.Version != 1 || old.Store != c.Store || old.Kind != kind || old.Query != c.Query || old.Before < 0 || old.High > c.High {
			return c, ErrCursor
		}
		if old.Floor != c.Floor {
			return c, ErrCursorExpired
		}
		c = old
	}
	return c, nil
}
func cursorString(c viewCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// SecretNames returns references, never environment values. It visits only source and
// destination specifications, not arbitrary condition values or observation fields.
func (t *Tx) SecretNames() (map[string][]string, error) {
	rows, err := t.sql.QueryContext(t.ctx, `SELECT DISTINCT w.id,j.value FROM watches w JOIN watch_revisions r ON r.watch_id=w.id AND r.revision=w.revision, json_tree(r.definition,'$.spec.source') j WHERE j.key='env' AND j.type='text' AND w.status!='deleted'
 UNION SELECT DISTINCT w.id,j.value FROM watches w JOIN watch_revisions r ON r.watch_id=w.id AND r.revision=w.revision, json_each(r.definition,'$.spec.destinations') target JOIN destinations d ON d.id=json_extract(target.value,'$.ref') JOIN destination_revisions dr ON dr.destination_id=d.id AND dr.revision=d.revision, json_tree(dr.definition,'$.spec') j WHERE j.key='env' AND j.type='text' AND w.status!='deleted' ORDER BY 1,2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var id, name string
		if err = rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = append(out[id], name)
	}
	return out, rows.Err()
}

const watchView = `WITH entity AS (SELECT watch_id,count(*) entities,coalesce(sum(json_extract(state,'$.open')),0) opened,coalesce(sum(json_extract(state,'$.sourceUnhealthy')),0) unhealthy FROM entities GROUP BY watch_id), delivery AS (SELECT watch_id,sum(status IN ('permanent','exhausted')) failed,sum(status IN ('pending','leased')) pending FROM outbox GROUP BY watch_id), summary AS (
 SELECT w.*,coalesce(json_extract(r.definition,'$.metadata.name'),'') name,json_extract(r.definition,'$.spec.source.type') source,json_extract(r.definition,'$.spec.policy.trigger') trigger,coalesce(json_extract(r.definition,'$.spec.condition.operator'),'') operator,coalesce(e.entities,0) entities,coalesce(e.opened,0) opened,coalesce(e.unhealthy,0) unhealthy,coalesce(d.failed,0) failed,coalesce(d.pending,0) pending,
 (coalesce(e.opened,0)>0 OR coalesce(e.unhealthy,0)>0 OR w.last_error!='' OR coalesce(d.failed,0)>0 OR w.id IN (SELECT value FROM json_each(?))) attention
 FROM watches w JOIN watch_revisions r ON r.watch_id=w.id AND r.revision=w.revision LEFT JOIN entity e ON e.watch_id=w.id LEFT JOIN delivery d ON d.watch_id=w.id)
 `

func (t *Tx) ConsoleWatches(q ConsoleQuery, missing map[string][]string) (WatchPage, error) {
	out := WatchPage{Watches: []ConsoleWatch{}, At: time.Now().UTC()}
	c, err := t.viewCursor("watches", q)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for id, names := range missing {
		if len(names) > 0 {
			ids = append(ids, id)
		}
	}
	raw, _ := json.Marshal(ids)
	where := ` WHERE (?='' OR instr(lower(id||' '||name),lower(?))>0) AND (?='' OR status=?) AND (?='' OR source=?) AND (?='' OR attention=1) AND (?='' OR id=?) AND (?='' OR (?='open' AND opened>0) OR (?='none' AND opened=0)) AND (?='' OR (?='error' AND (unhealthy>0 OR last_error!='')) OR (?='clear' AND unhealthy=0 AND last_error='')) AND (?='' OR (?='failed' AND failed>0) OR (?='pending' AND pending>0))`
	args := []any{string(raw), q.Search, q.Search, q.Status, q.Status, q.Source, q.Source, q.Attention, q.Watch, q.Watch, q.Incident, q.Incident, q.Incident, q.Health, q.Health, q.Health, q.Delivery, q.Delivery, q.Delivery}
	if err = t.sql.QueryRowContext(t.ctx, watchView+"SELECT count(*) FROM summary"+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if err = t.sql.QueryRowContext(t.ctx, watchView+"SELECT count(*),coalesce(sum(attention),0),coalesce(sum(status='paused'),0) FROM summary", string(raw)).Scan(&out.All, &out.Attention, &out.Paused); err != nil {
		return out, err
	}
	rows, err := t.sql.QueryContext(t.ctx, watchView+`SELECT id,name,revision,status,source,trigger,operator,generation,next_at,last_input_at,substr(last_error,1,2000),entities,opened,unhealthy,failed,pending FROM summary`+where+` AND id>? ORDER BY id LIMIT ?`, append(args, c.Key, q.Limit+1)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		if len(out.Watches) == q.Limit {
			out.More = true
			break
		}
		var v ConsoleWatch
		var next, last int64
		if err = rows.Scan(&v.ID, &v.Name, &v.Revision, &v.Status, &v.Source, &v.Trigger, &v.Operator, &v.Generation, &next, &last, &v.LastError, &v.Entities, &v.Open, &v.Unhealthy, &v.Failed, &v.Pending); err != nil {
			return out, err
		}
		v.NextAt = instant(next)
		v.LastInputAt = instant(last)
		v.Missing = missing[v.ID]
		if v.Missing == nil {
			v.Missing = []string{}
		}
		out.Watches = append(out.Watches, v)
		c.Key = v.ID
	}
	out.Cursor = cursorString(c)
	return out, rows.Err()
}
func eventFilter(q ConsoleQuery) (string, []any, error) {
	where := " WHERE (?='' OR watch_id=?) AND (?='' OR type=?)"
	args := []any{q.Watch, q.Watch, q.Type, q.Type}
	for _, bound := range []struct{ v, op string }{{q.From, ">="}, {q.To, "<="}} {
		if bound.v != "" {
			at, err := time.Parse(time.RFC3339, bound.v)
			if err != nil {
				return "", nil, ErrCursor
			}
			where += " AND at" + bound.op + "?"
			args = append(args, timestamp(at))
		}
	}
	return where, args, nil
}
func (t *Tx) ConsoleEvents(q ConsoleQuery) (EventSummaryPage, error) {
	out := EventSummaryPage{Events: []EventSummary{}, At: time.Now().UTC()}
	c, err := t.viewCursor("events", q)
	if err != nil {
		return out, err
	}
	where, args, err := eventFilter(q)
	if err != nil {
		return out, err
	}
	if err = t.sql.QueryRowContext(t.ctx, "SELECT count(*) FROM events"+where+" AND sequence<=?", append(args, c.High)...).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := t.sql.QueryContext(t.ctx, `SELECT id,sequence,watch_id,revision,type,at,coalesce(substr(json_extract(body,'$.message'),1,1000),''),coalesce(substr(json_extract(body,'$.entity'),1,500),'') FROM events`+where+" AND sequence<? AND sequence<=? ORDER BY sequence DESC LIMIT ?", append(args, c.Before, c.High, q.Limit+1)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		if len(out.Events) == q.Limit {
			out.More = true
			break
		}
		var v EventSummary
		var at int64
		if err = rows.Scan(&v.ID, &v.Sequence, &v.WatchID, &v.Revision, &v.Type, &at, &v.Message, &v.Entity); err != nil {
			return out, err
		}
		v.At = instant(at)
		out.Events = append(out.Events, v)
		c.Before = v.Sequence
	}
	out.Cursor = cursorString(c)
	raw, _ := json.Marshal(eventCursor{Version: 1, Store: c.Store, Watch: q.Watch, After: c.High})
	out.FollowCursor = base64.RawURLEncoding.EncodeToString(raw)
	return out, rows.Err()
}
func (t *Tx) ConsoleDeliveries(q ConsoleQuery) (DeliveryPage, error) {
	out := DeliveryPage{Deliveries: []ConsoleDelivery{}, At: time.Now().UTC()}
	c, err := t.viewCursor("deliveries", q)
	if err != nil {
		return out, err
	}
	where := ` WHERE (?='' OR watch_id=?) AND (?='' OR event_id=?) AND (?='' OR destination_id=?) AND (?='' OR status=?)`
	args := []any{q.Watch, q.Watch, q.Event, q.Event, q.Destination, q.Destination, q.Status, q.Status}
	if err = t.sql.QueryRowContext(t.ctx, "SELECT count(*) FROM outbox"+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	out.Counts, err = t.DeliveryCounts()
	if err != nil {
		return out, err
	}
	rows, err := t.sql.QueryContext(t.ctx, `SELECT id,event_id,watch_id,destination_id,destination_revision,status,attempts,next_at,created_at,substr(last_error,1,2000) FROM outbox`+where+" AND id<? ORDER BY id DESC LIMIT ?", append(args, c.Before, q.Limit+1)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		if len(out.Deliveries) == q.Limit {
			out.More = true
			break
		}
		var v ConsoleDelivery
		var next, created int64
		if err = rows.Scan(&v.ID, &v.EventID, &v.WatchID, &v.DestinationID, &v.DestinationRevision, &v.Status, &v.Attempts, &next, &created, &v.LastError); err != nil {
			return out, err
		}
		v.NextAt = instant(next)
		v.CreatedAt = instant(created)
		out.Deliveries = append(out.Deliveries, v)
		c.Before = v.ID
	}
	out.Cursor = cursorString(c)
	return out, rows.Err()
}
func (t *Tx) ConsoleDestinations(q ConsoleQuery) (DestinationPage, error) {
	out := DestinationPage{Destinations: []ConsoleDestination{}}
	c, err := t.viewCursor("destinations", q)
	if err != nil {
		return out, err
	}
	rows, err := t.sql.QueryContext(t.ctx, `SELECT r.definition,d.revision,(SELECT count(DISTINCT w.id) FROM watches w JOIN watch_revisions wr ON wr.watch_id=w.id AND wr.revision=w.revision,json_each(wr.definition,'$.spec.destinations') target WHERE w.status!='deleted' AND json_extract(target.value,'$.ref')=d.id) FROM destinations d JOIN destination_revisions r ON r.destination_id=d.id AND r.revision=d.revision WHERE d.id>? ORDER BY d.id LIMIT ?`, c.Key, q.Limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	size := 0
	for rows.Next() {
		var v ConsoleDestination
		var raw []byte
		if err = rows.Scan(&raw, &v.Revision, &v.Watches); err != nil {
			return out, err
		}
		if len(out.Destinations) == q.Limit || size+len(raw) > 4<<20 {
			out.More = true
			break
		}
		if err = json.Unmarshal(raw, &v.Definition); err != nil {
			return out, err
		}
		size += len(raw)
		out.Destinations = append(out.Destinations, v)
		c.Key = v.Definition.Metadata.ID
	}
	out.Cursor = cursorString(c)
	return out, rows.Err()
}

// ValidateConsoleQuery rejects misspelled filters instead of silently broadening a query.
func ValidateConsoleQuery(q ConsoleQuery) error {
	for _, f := range []struct{ value, allowed string }{{q.Status, "running paused deleted pending leased delivered permanent exhausted canceled"}, {q.Source, "http command push"}, {q.Attention, "yes"}, {q.Incident, "open none"}, {q.Health, "error clear"}, {q.Delivery, "failed pending"}} {
		if f.value != "" && !strings.Contains(" "+f.allowed+" ", " "+f.value+" ") {
			return ErrCursor
		}
	}
	return nil
}
