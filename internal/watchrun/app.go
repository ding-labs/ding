// Package watchrun owns applied configuration, transactions, and worker lifecycle.
package watchrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

type App struct {
	Store                               *store.Store
	HTTP                                source.HTTP
	Lookup                              source.Lookup
	Now                                 func() time.Time
	Output                              io.Writer
	AcquisitionWorkers, DeliveryWorkers int
	mu                                  sync.Mutex
	outputMu                            sync.Mutex
	running                             bool
	lastError                           string
}

func New(s *store.Store) *App {
	return &App{Store: s, HTTP: source.HTTP{Client: &http.Client{}, Lookup: os.LookupEnv}, Lookup: os.LookupEnv, Now: func() time.Time { return time.Now().UTC() }, Output: os.Stdout, AcquisitionWorkers: 32, DeliveryWorkers: 8}
}

type Change struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Previous string `json:"previous,omitempty"`
	State    string `json:"state"`
}
type ApplyRequest struct {
	Manifest string            `json:"manifest"`
	DryRun   bool              `json:"dryRun"`
	Expected map[string]string `json:"expected,omitempty"`
}
type ApplyResult struct {
	Changes []Change `json:"changes"`
	DryRun  bool     `json:"dryRun"`
}

func (a *App) Apply(ctx context.Context, request ApplyRequest) (ApplyResult, error) {
	result := ApplyResult{Changes: []Change{}, DryRun: request.DryRun}
	bundle, err := plan.Parse([]byte(request.Manifest))
	if err != nil {
		return result, err
	}
	for _, p := range bundle.Watches {
		if p.Definition.Spec.Source.Type != "http" || p.Definition.Spec.Source.JQ != "" {
			return result, fmt.Errorf("source capability is not implemented yet")
		}
		if _, err := condition.New(p.Definition, p.Revision); err != nil {
			return result, err
		}
	}
	for _, p := range bundle.Destinations {
		if p.Definition.Spec.Type != "webhook" && p.Definition.Spec.Type != "console" {
			return result, fmt.Errorf("destination capability is not implemented yet")
		}
	}
	now := a.Now()
	apply := func(tx *store.Tx) error {
		available := map[string]bool{}
		for _, d := range bundle.Destinations {
			available[d.Definition.Metadata.ID] = true
			if !request.DryRun {
				if err := tx.SaveDestination(d, now); err != nil {
					return err
				}
			}
		}
		for _, p := range bundle.Watches {
			id := p.Definition.Metadata.ID
			for _, target := range p.Definition.Spec.Destinations {
				if !available[target.Ref] {
					if _, err := tx.Destination(target.Ref, ""); err != nil {
						return fmt.Errorf("destination %s does not exist", target.Ref)
					}
				}
			}
			old, err := tx.Watch(id)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			previous := ""
			if err == nil {
				previous = old.Plan.Revision
			}
			if expected, ok := request.Expected[id]; ok && expected != previous {
				return store.ErrConflict
			}
			change := Change{ID: id, Revision: p.Revision, Previous: previous, State: "created"}
			if previous != "" {
				if previous != p.Revision {
					return fmt.Errorf("updating existing watches requires the lifecycle milestone")
				}
				change.State = "preserved"
			}
			result.Changes = append(result.Changes, change)
			if request.DryRun || previous != "" {
				continue
			}
			if err := tx.SaveWatch(store.WatchRecord{Plan: p, Generation: 1, Status: "running", NextAt: now}, now); err != nil {
				return err
			}
		}
		return nil
	}
	if request.DryRun {
		err = a.Store.View(ctx, apply)
	} else {
		err = a.Store.Update(ctx, apply)
	}
	return result, err
}

type Inspection struct {
	Watch      store.WatchRecord          `json:"watch"`
	Entities   map[string]json.RawMessage `json:"entities"`
	Deliveries []store.Intent             `json:"deliveries"`
}

func (a *App) Inspect(ctx context.Context, id string) (Inspection, error) {
	var result Inspection
	err := a.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		result.Watch, err = tx.Watch(id)
		if err != nil {
			return err
		}
		result.Entities, err = tx.Entities(id)
		if err != nil {
			return err
		}
		result.Deliveries, err = tx.Outbox(id)
		return err
	})
	return result, err
}
func (a *App) List(ctx context.Context) ([]store.WatchRecord, error) {
	var list []store.WatchRecord
	err := a.Store.View(ctx, func(tx *store.Tx) error { var err error; list, err = tx.Watches(); return err })
	return list, err
}

type Receipt struct {
	First     int64 `json:"first"`
	Last      int64 `json:"last"`
	Duplicate bool  `json:"duplicate"`
}

func (a *App) Accept(ctx context.Context, record store.WatchRecord, batch source.Batch, inputID string, now time.Time) (Receipt, error) {
	var receipt Receipt
	if inputID == "" || len(inputID) > 256 {
		return receipt, fmt.Errorf("invalid input identity")
	}
	if len(batch.Observations) == 0 || len(batch.Observations) > record.Plan.Definition.Spec.Limits.MaxOutputs {
		return receipt, fmt.Errorf("invalid observation count")
	}
	evaluator, err := condition.New(record.Plan.Definition, record.Plan.Revision)
	if err != nil {
		return receipt, err
	}
	err = a.Store.Update(ctx, func(tx *store.Tx) error {
		current, err := tx.Watch(record.Plan.Definition.Metadata.ID)
		if err != nil {
			return err
		}
		if current.Generation != record.Generation || current.Plan.Revision != record.Plan.Revision || current.Status != "running" {
			return store.ErrStale
		}
		id := current.Plan.Definition.Metadata.ID
		first, last, err := tx.Receipt(id, current.Generation, inputID, now)
		if err == nil {
			receipt = Receipt{First: first, Last: last, Duplicate: true}
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		interval, _ := time.ParseDuration(current.Plan.Definition.Spec.Source.Every)
		next := now.Add(interval)
		if batch.RetryAt.After(next) {
			next = batch.RetryAt
		}
		if err := tx.Checkpoint(id, current.Generation, next, now, batch.Cursor); err != nil {
			return err
		}
		states, err := tx.Entities(id)
		if err != nil {
			return err
		}
		for i, o := range batch.Observations {
			o.AcceptedAt = now
			o.InputID = inputID + ":" + strconv.Itoa(i)
			o.Sequence = 0
			sequence, err := tx.AppendObservation(id, current.Plan.Revision, current.Generation, o)
			if err != nil {
				return err
			}
			o.Sequence = sequence
			if receipt.First == 0 {
				receipt.First = sequence
			}
			receipt.Last = sequence
			keys := []string{}
			if o.Health != "ok" && len(states) > 0 {
				for key := range states {
					keys = append(keys, key)
				}
				sort.Strings(keys)
			} else {
				key, err := watch.EntityKey(current.Plan.Definition.Spec.GroupBy, o.Fields)
				if err != nil {
					return err
				}
				keys = append(keys, key)
			}
			for _, key := range keys {
				var state condition.State
				if data, ok := states[key]; ok {
					if err := json.Unmarshal(data, &state); err != nil {
						return err
					}
				} else if len(states) >= current.Plan.Definition.Spec.Limits.MaxEntities {
					return fmt.Errorf("entity budget exceeded")
				}
				result, err := evaluator.Evaluate(state, o, now)
				if err != nil {
					return err
				}
				if err := tx.SaveEntity(id, key, current.Plan.Revision, result.State, now); err != nil {
					return err
				}
				states[key], err = json.Marshal(result.State)
				if err != nil {
					return err
				}
				for _, event := range result.Events {
					sequence, err := tx.AppendEvent(event)
					if err != nil {
						return err
					}
					event.Sequence = sequence
					for _, target := range current.Plan.Definition.Spec.Destinations {
						if !contains(target.Events, event.Type) {
							continue
						}
						destination, err := tx.Destination(target.Ref, "")
						if err != nil {
							return err
						}
						payload, err := json.Marshal(watch.Envelope{APIVersion: watch.APIVersion, Data: event})
						if err != nil {
							return err
						}
						if _, err := tx.Enqueue(store.Intent{EventID: event.ID, WatchID: id, DestinationID: target.Ref, DestinationRevision: destination.Revision, Payload: payload, NextAt: now, CreatedAt: now}); err != nil {
							return err
						}
					}
				}
			}
		}
		return tx.SaveReceipt(id, current.Generation, inputID, receipt.First, receipt.Last, now.Add(24*time.Hour))
	})
	return receipt, err
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (a *App) note(err error) {
	if err != nil {
		a.mu.Lock()
		a.lastError = "runtime operation failed"
		a.mu.Unlock()
	}
}
func (a *App) Health() string { a.mu.Lock(); defer a.mu.Unlock(); return a.lastError }
