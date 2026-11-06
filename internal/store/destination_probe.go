package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

type DestinationProbe struct {
	ID      string    `json:"id"`
	Outcome string    `json:"outcome"`
	At      time.Time `json:"at"`
}

type destinationProbeRecord struct {
	DestinationProbe
	Digest string `json:"digest"`
}

var destinationProbeKey = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

// Reserve before external I/O. A pending receipt survives process loss and must
// never cause another send on replay. Metadata keeps this additive to old stores.
func (t *Tx) BeginDestinationProbe(id, digest string, now time.Time) (DestinationProbe, bool, error) {
	p := DestinationProbe{ID: id, Outcome: "pending", At: now}
	if !destinationProbeKey.MatchString(id) || len(digest) != 64 {
		return p, false, fmt.Errorf("invalid test identity")
	}
	var raw string
	err := t.sql.QueryRowContext(t.ctx, "SELECT value FROM metadata WHERE key=?", "destination_probe:"+id).Scan(&raw)
	if err == nil {
		var old destinationProbeRecord
		if err := json.Unmarshal([]byte(raw), &old); err != nil {
			return p, false, err
		}
		if old.Digest != digest {
			return p, false, ErrOperationConflict
		}
		return old.DestinationProbe, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return p, false, err
	}
	var count int
	if err := t.sql.QueryRowContext(t.ctx, "SELECT count(*) FROM metadata WHERE key GLOB 'destination_probe:*'").Scan(&count); err != nil {
		return p, false, err
	}
	if count >= 1000 {
		return p, false, fmt.Errorf("destination test receipt limit reached")
	}
	data, err := json.Marshal(destinationProbeRecord{p, digest})
	if err != nil {
		return p, false, err
	}
	_, err = t.sql.ExecContext(t.ctx, "INSERT INTO metadata(key,value) VALUES(?,?)", "destination_probe:"+id, string(data))
	return p, err == nil, err
}

func (t *Tx) FinishDestinationProbe(id, outcome string) error {
	if outcome != "accepted" && outcome != "failed" {
		return fmt.Errorf("invalid test outcome")
	}
	var raw string
	if err := t.sql.QueryRowContext(t.ctx, "SELECT value FROM metadata WHERE key=?", "destination_probe:"+id).Scan(&raw); err != nil {
		return err
	}
	var p destinationProbeRecord
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return err
	}
	if p.Outcome != "pending" {
		return ErrOperationConflict
	}
	p.Outcome = outcome
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(t.ctx, "UPDATE metadata SET value=? WHERE key=?", string(data), "destination_probe:"+id)
	return err
}
