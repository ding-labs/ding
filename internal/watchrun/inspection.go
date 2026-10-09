package watchrun

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/replay"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"gopkg.in/yaml.v3"
)

func (a *App) appendEvaluated(tx *store.Tx, r store.WatchRecord, e watch.Event, now time.Time, c replay.Checkpoint) error {
	if err := a.appendEvent(tx, r, e, now); err != nil {
		return err
	}
	// A new-event firing depends only on this ID's absence and available
	// capacity. Copying every unrelated ID into every firing checkpoint would
	// make durable history quadratic in the dedup horizon.
	if r.Plan.Definition.Spec.Condition.Operator == "new-event" && e.Type == "firing" {
		active := 0
		for _, expiry := range c.Prior.Seen {
			if now.Before(expiry) {
				active++
			}
		}
		if active < r.Plan.Definition.Spec.Limits.MaxSamples {
			id, _ := c.Input.Fields[r.Plan.Definition.Spec.Condition.Field].(string)
			seen := map[string]time.Time{}
			if expiry, ok := c.Prior.Seen[id]; ok {
				seen[id] = expiry
			}
			c.Prior.Seen = seen
		}
	}
	return tx.SaveReplay(e.ID, c)
}
func (a *App) Evidence(ctx context.Context, id string) (replay.Evidence, error) {
	proof := replay.Evidence{}
	err := a.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		proof.Event, err = tx.Event(id)
		if err != nil {
			return err
		}
		proof.Definition, err = tx.Definition(proof.Event.WatchID, proof.Event.Revision)
		if err != nil {
			return err
		}
		raw, err := tx.Replay(id)
		if errors.Is(err, store.ErrNotFound) {
			proof.ReplayStatus = "unavailable: lifecycle event or predates schema 2"
			return nil
		}
		if err != nil {
			return err
		}
		proof.Checkpoint = &replay.Checkpoint{}
		if err := json.Unmarshal(raw, proof.Checkpoint); err != nil {
			return err
		}
		if err := replay.Verify(proof); err != nil {
			return err
		}
		proof.ReplayStatus = "verified"
		return nil
	})
	return proof, err
}
func (a *App) Export(ctx context.Context, id string) (string, error) {
	var out strings.Builder
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	err := a.Store.View(ctx, func(tx *store.Tx) error {
		record, err := tx.Watch(id)
		if err != nil {
			return err
		}
		if err := enc.Encode(record.Plan.Definition); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, ref := range record.Plan.Definition.Spec.Destinations {
			if seen[ref.Ref] {
				continue
			}
			seen[ref.Ref] = true
			d, err := tx.Destination(ref.Ref, "")
			if err != nil {
				return err
			}
			if err := enc.Encode(d.Definition); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return out.String(), nil
}
func (a *App) Retry(ctx context.Context, id int64) error {
	return a.Store.Update(ctx, func(tx *store.Tx) error {
		if a.isClosing() {
			return ErrClosing
		}
		usage, err := tx.Budget()
		if err != nil {
			return err
		}
		if usage.Pending >= a.Limits.MaxPending || usage.Bytes >= a.Limits.MaxBytes {
			return ErrQuota
		}
		return tx.Retry(id, a.Now())
	})
}

type SourceHealth struct {
	ID                string    `json:"id"`
	Status            string    `json:"status"`
	LastInputAt       time.Time `json:"lastInputAt"`
	NextAt            time.Time `json:"nextAt"`
	LastError         string    `json:"lastError,omitempty"`
	UnhealthyEntities int       `json:"unhealthyEntities"`
	OpenIncidents     int       `json:"openIncidents"`
}
type CredentialHealth struct {
	Environment string `json:"environment"`
	Present     bool   `json:"present"`
}
type Doctor struct {
	Healthy          bool               `json:"healthy"`
	Store            store.Health       `json:"store"`
	Usage            store.Usage        `json:"usage"`
	Limits           Limits             `json:"limits"`
	Running          bool               `json:"running"`
	Closing          bool               `json:"closing"`
	Acquisitions     int                `json:"acquisitions"`
	AcquisitionLimit int                `json:"acquisitionLimit"`
	DeliveryLimit    int                `json:"deliveryLimit"`
	LastError        string             `json:"lastError,omitempty"`
	Sources          []SourceHealth     `json:"sources"`
	Credentials      []CredentialHealth `json:"credentials"`
	Deliveries       map[string]int     `json:"deliveries"`
}

func (a *App) Doctor(ctx context.Context) (Doctor, error) {
	d := Doctor{Healthy: true, Limits: a.Limits, Sources: []SourceHealth{}, Credentials: []CredentialHealth{}, AcquisitionLimit: a.AcquisitionWorkers, DeliveryLimit: a.DeliveryWorkers}
	var err error
	d.Store, err = a.Store.Health(ctx)
	if err != nil {
		return d, err
	}
	a.mu.Lock()
	d.Running = a.running
	d.Closing = a.closing
	d.Acquisitions = a.inflight
	d.LastError = a.lastError
	a.mu.Unlock()
	refs := map[string]bool{}
	add := func(ref *watch.SecretRef) {
		if ref != nil {
			refs[ref.Env] = true
		}
	}
	err = a.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		d.Usage, err = tx.Usage()
		if err != nil {
			return err
		}
		d.Deliveries, err = tx.DeliveryCounts()
		if err != nil {
			return err
		}
		records, err := tx.Watches()
		if err != nil {
			return err
		}
		for _, record := range records {
			if record.Status == "deleted" {
				continue
			}
			s := record.Plan.Definition.Spec.Source
			add(s.URLRef)
			for _, ref := range s.Headers {
				add(&ref)
			}
			for _, ref := range s.Env {
				add(&ref)
			}
			status := SourceHealth{ID: record.Plan.Definition.Metadata.ID, Status: record.Status, LastInputAt: record.LastInputAt, NextAt: record.NextAt, LastError: record.LastError}
			status.UnhealthyEntities, status.OpenIncidents, err = tx.EntityHealth(status.ID)
			if err != nil {
				return err
			}
			if status.LastError != "" || status.UnhealthyEntities > 0 {
				d.Healthy = false
			}
			d.Sources = append(d.Sources, status)
		}
		destinations, err := tx.Destinations()
		if err != nil {
			return err
		}
		for _, dest := range destinations {
			add(dest.Spec.URLRef)
			for _, ref := range dest.Spec.Headers {
				add(&ref)
			}
		}
		return nil
	})
	if err != nil {
		return d, err
	}
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value, ok := a.Lookup(name)
		present := ok && value != ""
		d.Credentials = append(d.Credentials, CredentialHealth{name, present})
		if !present {
			d.Healthy = false
		}
	}
	if !d.Running || d.Closing || d.Store.Integrity != "ok" || d.Usage.Bytes >= d.Limits.MaxBytes || d.Usage.Pending >= d.Limits.MaxPending || d.Deliveries["permanent"]+d.Deliveries["exhausted"] > 0 {
		d.Healthy = false
	}
	return d, nil
}
