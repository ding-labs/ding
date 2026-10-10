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
	"strconv"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/notify"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/replay"
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
	Notify                              func(context.Context, notify.Message) error
	ValidateBundle                      func(plan.Bundle) error // Optional execution policy; immutable after startup.
	AcquisitionWorkers, DeliveryWorkers int
	PollInterval                        time.Duration // Zero preserves the local 100ms scheduler cadence.
	mu                                  sync.Mutex
	outputMu                            sync.Mutex
	outputPermit                        chan struct{}
	running                             bool
	lastError                           string
	closing                             bool
	inflight                            int
	cancels                             map[string]acquisition
	Limits                              Limits
}

func New(s *store.Store) *App {
	return &App{Store: s, HTTP: source.HTTP{Client: &http.Client{}, Lookup: os.LookupEnv}, Lookup: os.LookupEnv, Now: func() time.Time { return time.Now().UTC() }, Output: os.Stdout, AcquisitionWorkers: 32, DeliveryWorkers: 8, Limits: DefaultLimits(), cancels: map[string]acquisition{}}
}

type Change struct {
	Kind        string   `json:"kind"`
	Before      string   `json:"before,omitempty"`
	After       string   `json:"after,omitempty"`
	Permissions []string `json:"permissions"`
	ID          string   `json:"id"`
	Revision    string   `json:"revision"`
	Previous    string   `json:"previous,omitempty"`
	State       string   `json:"state"`
}
type ApplyRequest struct {
	Review   *ApplyPreconditions `json:"review,omitempty"`
	Manifest string              `json:"manifest"`
	DryRun   bool                `json:"dryRun"`
	Expected map[string]string   `json:"expected,omitempty"`
}
type ApplyResult struct {
	DestinationChanges []Change            `json:"destinationChanges"`
	Review             *ApplyPreconditions `json:"review"`
	Credentials        []CredentialHealth  `json:"credentials"`
	Changes            []Change            `json:"changes"`
	DryRun             bool                `json:"dryRun"`
}

func (a *App) Apply(ctx context.Context, request ApplyRequest) (ApplyResult, error) {
	return a.apply(ctx, request, nil)
}

func (a *App) apply(ctx context.Context, request ApplyRequest, mutation mutation) (ApplyResult, error) {
	if mutation == nil {
		mutation = noMutation{}
	}
	result := ApplyResult{Changes: []Change{}, DestinationChanges: []Change{}, Credentials: []CredentialHealth{}, DryRun: request.DryRun}
	bundle, err := plan.Parse([]byte(request.Manifest))
	if err != nil {
		return result, err
	}
	if a.ValidateBundle != nil {
		if err := a.ValidateBundle(bundle); err != nil {
			return result, err
		}
	}
	for _, p := range bundle.Watches {

		if _, err := condition.New(p.Definition, p.Revision); err != nil {
			return result, err
		}
	}
	now := a.Now()
	changed := map[string]int64{}
	apply := func(tx *store.Tx) error {
		if done, err := mutation.begin(tx, &result, a.Now()); done || err != nil {
			return err
		}
		if a.isClosing() {
			return ErrClosing
		}
		usage, err := tx.Budget()
		if err != nil {
			return err
		}
		result.Review, err = captureReview(tx, bundle, request.Manifest)
		if err != nil {
			return err
		}
		if request.Review != nil && !sameReview(request.Review, result.Review) {
			return store.ErrConflict
		}
		extra := []watch.Destination{}
		bundled := map[string]bool{}
		for _, d := range bundle.Destinations {
			bundled[d.Definition.Metadata.ID] = true
		}
		for id := range result.Review.Destinations {
			if bundled[id] {
				continue
			}
			if old, e := tx.Destination(id, ""); e == nil {
				extra = append(extra, old.Definition)
			}
		}
		result.Credentials = a.BundleCredentials(bundle, extra)
		if a.Limits.MaxDestinations > 0 {
			existing, err := tx.Destinations()
			if err != nil {
				return err
			}
			ids := map[string]bool{}
			for _, d := range existing {
				ids[d.Metadata.ID] = true
			}
			for _, d := range bundle.Destinations {
				ids[d.Definition.Metadata.ID] = true
			}
			if len(ids) > a.Limits.MaxDestinations {
				return ErrQuota
			}
		}
		available := map[string]bool{}
		for _, d := range bundle.Destinations {
			available[d.Definition.Metadata.ID] = true
			old, e := tx.Destination(d.Definition.Metadata.ID, "")
			if e != nil && !errors.Is(e, store.ErrNotFound) {
				return e
			}
			change := Change{Kind: "Destination", ID: d.Definition.Metadata.ID, Revision: d.Revision, Previous: old.Revision, State: "created", After: yamlDefinition(d.Definition), Permissions: []string{}}
			if old.Revision != "" {
				change.State = "updated"
				change.Before = yamlDefinition(old.Definition)
				if old.Revision == d.Revision {
					change.State = "preserved"
				}
			}
			result.DestinationChanges = append(result.DestinationChanges, change)
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
			change := Change{Kind: "Watch", After: yamlDefinition(p.Definition), ID: id, Revision: p.Revision, Previous: previous, State: "created", Permissions: p.Permissions}
			record := store.WatchRecord{Plan: p, Generation: 1, Status: "running", NextAt: now}
			reset := false
			if previous != "" {
				change.Before = yamlDefinition(old.Plan.Definition)
				record = old
				record.Plan = p
				if old.Status == "deleted" {
					return fmt.Errorf("watch %s is deleted; use a new watch ID", id)
				}
				change.State = "preserved"
				if previous != p.Revision {
					record.Generation++
					reset = old.Plan.Fingerprint != p.Fingerprint
					if reset {
						change.State = "reset"
						record.Cursor = ""
						record.LastInputAt = time.Time{}
						record.LastError = ""
						record.NextAt = now
					}
				}
			} else {
				usage.Watches++
				if usage.Watches > a.Limits.MaxWatches {
					return ErrQuota
				}
			}
			result.Changes = append(result.Changes, change)
			if request.DryRun || previous == p.Revision {
				continue
			}
			if err := tx.SaveWatch(record, now); err != nil {
				return err
			}
			if reset {
				if err := tx.ResetState(id); err != nil {
					return err
				}
				if err := lifecycleEvent(tx, record, "state_reset", "source, condition, grouping, policy, or limits changed", now); err != nil {
					return err
				}
			}
			if previous != "" && !reset {
				if err := tx.RevisionState(id, p.Revision, record.Generation); err != nil {
					return err
				}
			} else {
				if err := armInitial(tx, record, now); err != nil {
					return err
				}
			}
			if err := lifecycleEvent(tx, record, "applied", change.State, now); err != nil {
				return err
			}
			changed[id] = record.Generation

		}
		if !request.DryRun {
			usage, err := tx.Budget()
			if err != nil {
				return err
			}
			if usage.Bytes > a.Limits.MaxBytes {
				return ErrQuota
			}
		}
		if err := reviewSize(result); err != nil {
			return err
		}
		return mutation.finish(tx, result, a.Now())
	}
	if request.DryRun {
		err = a.Store.View(ctx, apply)
	} else {
		err = a.Store.Update(ctx, apply)
	}
	if err == nil && !request.DryRun {
		for id, generation := range changed {
			a.cancelBefore(id, generation)
		}
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
	return a.accept(ctx, record, batch, inputID, func() time.Time { return now })
}

// Live time is sampled after obtaining the transaction lock. Concurrent push
// requests cannot commit timestamps in the reverse order of their evaluation.
func (a *App) accept(ctx context.Context, record store.WatchRecord, batch source.Batch, inputID string, clock func() time.Time) (Receipt, error) {
	var receipt Receipt
	if a.isClosing() {
		return receipt, ErrClosing
	}
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
		now := clock()
		if a.isClosing() {
			return ErrClosing
		}
		current, err := tx.Watch(record.Plan.Definition.Metadata.ID)
		if err != nil {
			return err
		}
		if current.Generation != record.Generation || current.Plan.Revision != record.Plan.Revision || current.Status != "running" {
			return store.ErrStale
		}
		id := current.Plan.Definition.Metadata.ID
		timerInput := !batch.Deadline.IsZero()
		if timerInput {
			timer, err := tx.Timer(id, batch.Entity, "missing")
			if err != nil {
				return store.ErrStale
			}
			if timer.Generation != current.Generation || !timer.Due.Equal(batch.Deadline) || now.Before(timer.Due) {
				return store.ErrStale
			}
			if err := tx.DeleteTimer(timer); err != nil {
				return err
			}
		}
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
		if !timerInput {
			if err := tx.Checkpoint(id, current.Generation, next, now, batch.Cursor); err != nil {
				return err
			}
		}
		usage, err := tx.Budget()
		if err != nil {
			return err
		}
		if usage.Bytes >= a.Limits.MaxBytes || usage.Pending >= a.Limits.MaxPending {
			return ErrQuota
		}
		if current.LastError != "" && !timerInput {
			batch.Observations = append([]watch.Observation{{Health: "gap", Detail: "backpressure_gap"}}, batch.Observations...)
		}
		states, err := tx.Entities(id)
		if err != nil {
			return err
		}
		for i, o := range batch.Observations {
			o.AcceptedAt = now
			o.InputID = inputID + ":" + strconv.Itoa(i) + ":" + strconv.FormatInt(now.UnixNano(), 10)
			o.Sequence = 0
			if timerInput {
				o.Entity = batch.Entity
				deadline := batch.Deadline
				o.Deadline = &deadline
			}
			encoded, err := json.Marshal(o)
			if err != nil {
				return err
			}
			if len(encoded) > current.Plan.Definition.Spec.Limits.MaxBytes {
				return ErrQuota
			}
			sequence, err := tx.AppendObservation(id, current.Plan.Revision, current.Generation, o)
			if err != nil {
				return err
			}
			o.Sequence = sequence
			if receipt.First == 0 {
				receipt.First = sequence
			}
			receipt.Last = sequence
			existing := make([]string, 0, len(states))
			for key := range states {
				existing = append(existing, key)
			}
			keys, clearSource, err := evaluator.Route(existing, o)
			if err != nil {
				return err
			}
			if clearSource {
				var bootstrap condition.State
				if err := json.Unmarshal(states[condition.SourceEntity], &bootstrap); err != nil {
					return err
				}
				if err := tx.DeleteEntity(id, condition.SourceEntity); err != nil {
					return err
				}
				delete(states, condition.SourceEntity)
				if bootstrap.SourceUnhealthy {
					if err := a.appendEvaluated(tx, current, evaluator.SourceRecovered(o), now, replay.Checkpoint{Prior: bootstrap, Input: o, SourceRecovered: true}); err != nil {
						return err
					}
				}
			}

			for _, key := range keys {
				state := condition.State{Entity: key}
				if data, ok := states[key]; ok {
					if err := json.Unmarshal(data, &state); err != nil {
						return err
					}
				} else if len(states) >= current.Plan.Definition.Spec.Limits.MaxEntities {
					return ErrQuota
				}
				result, err := evaluator.Evaluate(state, o, now)
				if err != nil {
					return err
				}
				if err := tx.SaveEntity(id, key, current.Plan.Revision, result.State, now); err != nil {
					return err
				}
				if current.Plan.Definition.Spec.Condition.MissingFor != "" && (o.Health == "ok" || o.Health == "unchanged") {
					if err := tx.SaveTimer(store.Timer{WatchID: id, Entity: key, Kind: "missing", Generation: current.Generation, Due: result.State.MissingAt}); err != nil {
						return err
					}
				}
				states[key], err = json.Marshal(result.State)
				if err != nil {
					return err
				}
				for _, event := range result.Events {
					if err := a.appendEvaluated(tx, current, event, now, replay.Checkpoint{Prior: state, Input: o}); err != nil {
						return err
					}
				}

			}
		}
		usage, err = tx.Budget()
		if err != nil {
			return err
		}
		if usage.Bytes > a.Limits.MaxBytes {
			return ErrQuota
		}
		return tx.SaveReceipt(id, current.Generation, inputID, receipt.First, receipt.Last, now.Add(24*time.Hour))
	})
	if err != nil && !errors.Is(err, store.ErrStale) && !errors.Is(err, ErrClosing) && ctx.Err() == nil {
		detail := "acceptance_failed"
		if errors.Is(err, ErrQuota) {
			detail = "quota_exceeded"
		}
		a.note(err)
		_ = a.Store.Update(ctx, func(tx *store.Tx) error {
			return tx.LastError(record.Plan.Definition.Metadata.ID, record.Generation, detail, time.Time{})
		})
	}
	if err == nil && !receipt.Duplicate && receipt.Last > 0 {
		a.mu.Lock()
		a.lastError = ""
		a.mu.Unlock()
	}
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
	if err != nil && !errors.Is(err, context.Canceled) {
		a.mu.Lock()
		a.lastError = "runtime operation failed"
		a.mu.Unlock()
	}
}
func (a *App) Health() string { a.mu.Lock(); defer a.mu.Unlock(); return a.lastError }
