package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

type Handoff struct {
	ID           string            `json:"id"`
	WatchID      string            `json:"watchId"`
	Role         string            `json:"role"`
	Phase        string            `json:"phase"`
	Peer         string            `json:"peer"`
	Digest       string            `json:"digest"`
	Revision     string            `json:"revision"`
	Generation   int64             `json:"generation"`
	Destinations map[string]string `json:"destinations"`
	Held         bool              `json:"held"`
	At           time.Time         `json:"at"`
}

var handoffID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (t *Tx) Handoff(id string) (Handoff, error) {
	var h Handoff
	var b []byte
	err := t.sql.QueryRowContext(t.ctx, "SELECT body FROM handoffs WHERE id=?", id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return h, ErrNotFound
	}
	if err != nil {
		return h, err
	}
	return h, json.Unmarshal(b, &h)
}
func (t *Tx) Handoffs() ([]Handoff, error) {
	rows, err := t.sql.QueryContext(t.ctx, "SELECT body FROM handoffs ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Handoff{}
	for rows.Next() {
		var b []byte
		var h Handoff
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &h); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, rows.Err()
}
func (t *Tx) HandoffHold(watchID string) (string, error) {
	var id string
	err := t.sql.QueryRowContext(t.ctx, "SELECT id FROM handoffs WHERE watch_id=? AND held=1", watchID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
func (t *Tx) SaveHandoff(h Handoff) error {
	if !handoffID.MatchString(h.ID) || h.WatchID == "" || len(h.Peer) > 1024 || len(h.Destinations) > 100 {
		return fmt.Errorf("invalid handoff")
	}
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	if len(b) > 64<<10 {
		return fmt.Errorf("handoff exceeds limit")
	}
	var n int
	if err := t.sql.QueryRowContext(t.ctx, "SELECT COUNT(*) FROM handoffs WHERE id!=?", h.ID).Scan(&n); err != nil {
		return err
	}
	if n >= 1000 {
		return fmt.Errorf("handoff history limit reached")
	}
	_, err = t.sql.ExecContext(t.ctx, "INSERT INTO handoffs(id,watch_id,held,body) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET held=excluded.held,body=excluded.body", h.ID, h.WatchID, h.Held, b)
	return err
}
func (t *Tx) PendingForWatch(id string) (int, error) {
	var n int
	err := t.sql.QueryRowContext(t.ctx, "SELECT count(*) FROM outbox WHERE watch_id=? AND status IN ('pending','leased')", id).Scan(&n)
	return n, err
}
