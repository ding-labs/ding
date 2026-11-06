package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// DestinationSetup is private binding material, never a model response. Digest
// covers the resolved credentials as well as the saved destination definition.
type DestinationSetup struct {
	Account     string `json:"account"`
	Workspace   string `json:"workspace"`
	Destination string `json:"destination"`
	Revision    string `json:"revision"`
	Digest      string `json:"digest"`
}

const DestinationConfirmationTTL = 24 * time.Hour

type DestinationReadiness struct {
	State       string            `json:"state"`
	Probe       *DestinationProbe `json:"probe,omitempty"`
	ConfirmedAt *time.Time        `json:"confirmedAt,omitempty"`
	ExpiresAt   *time.Time        `json:"expiresAt,omitempty"`
}

func (t *Tx) bindDestinationProbe(id, digest string, setup DestinationSetup) error {
	var raw string
	if err := t.sql.QueryRowContext(t.ctx, t.query(qDestinationProbeRead), "destination_probe:"+id).Scan(&raw); err != nil {
		return err
	}
	var record destinationProbeRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return err
	}
	if record.Digest != digest || setup.Account == "" || setup.Workspace == "" || setup.Destination == "" || setup.Revision == "" || len(setup.Digest) != 64 {
		return ErrOperationConflict
	}
	if record.Setup != nil && *record.Setup != setup {
		return ErrOperationConflict
	}
	record.Setup = &setup
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = t.exec(qDestinationProbeFinish, string(data), "destination_probe:"+id)
	return err
}

// BeginSetupProbe reserves and binds the receipt in the same transaction before I/O.
func (t *Tx) BeginSetupProbe(id, digest string, setup DestinationSetup, now time.Time) (DestinationProbe, bool, error) {
	p, start, err := t.BeginDestinationProbe(id, digest, now)
	if err == nil {
		err = t.bindDestinationProbe(id, digest, setup)
	}
	return p, start, err
}

func (t *Tx) latestDestinationProbe(setup DestinationSetup) (destinationProbeRecord, error) {
	rows, err := t.sql.QueryContext(t.ctx, t.query(qDestinationProbes))
	if err != nil {
		return destinationProbeRecord{}, err
	}
	defer rows.Close()
	var latest destinationProbeRecord
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return latest, err
		}
		var p destinationProbeRecord
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return latest, err
		}
		if p.Setup == nil || p.Setup.Account != setup.Account || p.Setup.Workspace != setup.Workspace || p.Setup.Destination != setup.Destination {
			continue
		}
		if latest.ID == "" || p.At.After(latest.At) || p.At.Equal(latest.At) && p.ID > latest.ID {
			latest = p
		}
	}
	if err := rows.Err(); err != nil {
		return latest, err
	}
	if latest.ID == "" {
		return latest, sql.ErrNoRows
	}
	return latest, nil
}

func readiness(p destinationProbeRecord, setup DestinationSetup, now time.Time) DestinationReadiness {
	out := DestinationReadiness{State: p.Outcome, Probe: &p.DestinationProbe}
	expires := p.At.Add(DestinationConfirmationTTL)
	out.ExpiresAt = &expires
	if p.Setup == nil || *p.Setup != setup {
		out.State = "stale"
		return out
	}
	if now.Before(p.At) || !now.Before(expires) {
		out.State = "expired"
		return out
	}
	if p.Outcome == "accepted" && p.ConfirmedAt != nil {
		out.State = "confirmed"
		out.ConfirmedAt = p.ConfirmedAt
	}
	return out
}

func (t *Tx) DestinationReadiness(setup DestinationSetup, now time.Time) (DestinationReadiness, error) {
	p, err := t.latestDestinationProbe(setup)
	if errors.Is(err, sql.ErrNoRows) {
		return DestinationReadiness{State: "untested"}, nil
	}
	if err != nil {
		return DestinationReadiness{}, err
	}
	return readiness(p, setup, now), nil
}

func (t *Tx) ConfirmDestinationProbe(id string, setup DestinationSetup, now time.Time) (DestinationReadiness, error) {
	p, err := t.latestDestinationProbe(setup)
	if err != nil {
		return DestinationReadiness{}, err
	}
	state := readiness(p, setup, now)
	if p.ID != id || (state.State != "accepted" && state.State != "confirmed") {
		return DestinationReadiness{}, ErrConflict
	}
	if p.ConfirmedAt == nil {
		p.ConfirmedAt = &now
		data, err := json.Marshal(p)
		if err != nil {
			return DestinationReadiness{}, err
		}
		if _, err := t.exec(qDestinationProbeFinish, string(data), "destination_probe:"+id); err != nil {
			return DestinationReadiness{}, err
		}
	}
	return readiness(p, setup, now), nil
}

// Receipt creation is capped at 1000 per workspace; no unbounded history scan.
var qDestinationProbes = statement{
	sqlite:   "SELECT value FROM metadata WHERE key GLOB 'destination_probe:*' LIMIT 1000",
	postgres: "SELECT value FROM metadata WHERE starts_with(key,'destination_probe:') LIMIT 1000",
}
