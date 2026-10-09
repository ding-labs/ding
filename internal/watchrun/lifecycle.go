package watchrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/replay"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

var ErrQuota = errors.New("resource quota exceeded")
var ErrClosing = errors.New("runtime is shutting down")

type Limits struct {
	MaxWatches int           `json:"maxWatches"`
	MaxPending int           `json:"maxPending"`
	MaxBytes   int64         `json:"maxBytes"`
	Retention  time.Duration `json:"retention"`
}

func DefaultLimits() Limits {
	return Limits{MaxWatches: 1000, MaxPending: 10000, MaxBytes: 1 << 30, Retention: 7 * 24 * time.Hour}
}

type acquisition struct {
	generation int64
	cancel     context.CancelFunc
}

func (a *App) isClosing() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.closing }
func (a *App) cancelBefore(id string, generation int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if active, ok := a.cancels[id]; ok && active.generation < generation {
		active.cancel()
	}
}
func lifecycleEvent(tx *store.Tx, r store.WatchRecord, kind, reason string, now time.Time) error {
	id, err := watch.Revision(struct {
		Watch, Revision, Kind string
		Generation            int64
		At                    time.Time
	}{r.Plan.Definition.Metadata.ID, r.Plan.Revision, kind, r.Generation, now})
	if err != nil {
		return err
	}
	_, err = tx.AppendEvent(watch.Event{ID: id, WatchID: r.Plan.Definition.Metadata.ID, Revision: r.Plan.Revision, Type: kind, At: now, Message: reason})
	return err
}
func armInitial(tx *store.Tx, r store.WatchRecord, now time.Time) error {
	if r.Status != "running" || r.Plan.Definition.Spec.Condition.MissingFor == "" || len(r.Plan.Definition.Spec.GroupBy) > 0 {
		return nil
	}
	duration, _ := time.ParseDuration(r.Plan.Definition.Spec.Condition.MissingFor)
	key, _ := watch.EntityKey(nil, nil)
	state := condition.State{Entity: key, LastAt: now, MissingAt: now.Add(duration)}
	if err := tx.SaveEntity(r.Plan.Definition.Metadata.ID, key, r.Plan.Revision, state, now); err != nil {
		return err
	}
	return tx.SaveTimer(store.Timer{WatchID: r.Plan.Definition.Metadata.ID, Entity: key, Kind: "missing", Generation: r.Generation, Due: state.MissingAt})
}
func (a *App) appendEvent(tx *store.Tx, r store.WatchRecord, event watch.Event, now time.Time) error {
	sequence, err := tx.AppendEvent(event)
	if err != nil {
		return err
	}
	event.Sequence = sequence
	for _, target := range r.Plan.Definition.Spec.Destinations {
		if !contains(target.Events, event.Type) {
			continue
		}
		pending, err := tx.Pending()
		if err != nil {
			return err
		}
		if pending >= a.Limits.MaxPending {
			return ErrQuota
		}
		destination, err := tx.Destination(target.Ref, "")
		if err != nil {
			return err
		}
		payload, err := delivery.Render(destination.Definition.Spec.Type, event)
		if err != nil {
			return err
		}
		if _, err := tx.Enqueue(store.Intent{EventID: event.ID, WatchID: r.Plan.Definition.Metadata.ID, DestinationID: target.Ref, DestinationRevision: destination.Revision, Payload: payload, NextAt: now, CreatedAt: now}); err != nil {
			return err
		}
	}
	return nil
}

type LifecycleRequest struct {
	Action        string `json:"action"`
	Expected      string `json:"expected,omitempty"`
	CancelPending bool   `json:"cancelPending,omitempty"`
}

func (a *App) Lifecycle(ctx context.Context, id string, request LifecycleRequest) (store.WatchRecord, error) {
	var record store.WatchRecord
	now := a.Now()
	err := a.Store.Update(ctx, func(tx *store.Tx) error {
		if a.isClosing() {
			return ErrClosing
		}
		var err error
		record, err = tx.Watch(id)
		if err != nil {
			return err
		}
		if request.Expected != "" && request.Expected != record.Plan.Revision {
			return store.ErrConflict
		}
		if request.CancelPending && request.Action != "delete" {
			return fmt.Errorf("cancelPending requires delete")
		}
		status := ""
		switch request.Action {
		case "pause":
			status = "paused"
		case "resume":
			status = "running"
		case "delete":
			status = "deleted"
		default:
			return fmt.Errorf("unknown lifecycle action")
		}
		if record.Status == "deleted" && status != "deleted" {
			return fmt.Errorf("deleted watches cannot resume; use a new watch ID")
		}
		if request.CancelPending {
			if err := tx.CancelDeliveries(id, now); err != nil {
				return err
			}
		}
		if record.Status == status {
			return nil
		}
		record.Status = status
		record.Generation++
		record.NextAt = now
		record.LastError = ""
		if err := tx.SaveWatch(record, now); err != nil {
			return err
		}
		if err := tx.DeleteTimers(id); err != nil {
			return err
		}
		if status == "running" {
			states, err := tx.Entities(id)
			if err != nil {
				return err
			}
			e, err := condition.New(record.Plan.Definition, record.Plan.Revision)
			if err != nil {
				return err
			}
			for key, data := range states {
				var old condition.State
				if err := json.Unmarshal(data, &old); err != nil {
					return err
				}
				o := watch.Observation{InputID: fmt.Sprintf("resume:%d:%s", record.Generation, key), AcceptedAt: now, Health: "gap", Detail: "resumed"}
				o.Sequence, err = tx.AppendObservation(id, record.Plan.Revision, record.Generation, o)
				if err != nil {
					return err
				}
				result, err := e.Evaluate(old, o, now)
				if err != nil {
					return err
				}
				if record.Plan.Definition.Spec.Condition.MissingFor != "" {
					duration, _ := time.ParseDuration(record.Plan.Definition.Spec.Condition.MissingFor)
					result.State.MissingAt = now.Add(duration)
					if err := tx.SaveTimer(store.Timer{WatchID: id, Entity: key, Kind: "missing", Generation: record.Generation, Due: result.State.MissingAt}); err != nil {
						return err
					}
				}
				if err := tx.SaveEntity(id, key, record.Plan.Revision, result.State, now); err != nil {
					return err
				}
				for _, event := range result.Events {
					if err := a.appendEvaluated(tx, record, event, now, replay.Checkpoint{Prior: old, Input: o}); err != nil {
						return err
					}
				}
			}
			if len(states) == 0 {
				if err := armInitial(tx, record, now); err != nil {
					return err
				}
			}
		}
		return lifecycleEvent(tx, record, request.Action, request.Action, now)
	})
	if err == nil {
		a.cancelBefore(id, record.Generation)
	}
	return record, err
}
func (a *App) RunTimers(ctx context.Context, now time.Time) error {
	var timers []store.Timer
	if err := a.Store.View(ctx, func(tx *store.Tx) error { var err error; timers, err = tx.DueTimers(now, 100); return err }); err != nil {
		return err
	}
	for _, timer := range timers {
		var r store.WatchRecord
		if err := a.Store.View(ctx, func(tx *store.Tx) error { var err error; r, err = tx.Watch(timer.WatchID); return err }); err != nil {
			return err
		}
		key, _ := watch.Revision(timer.Entity)
		_, err := a.Accept(ctx, r, source.Batch{Entity: timer.Entity, Deadline: timer.Due, Observations: []watch.Observation{{Health: "timer"}}}, fmt.Sprintf("timer:%d:%s:%d", timer.Generation, key, timer.Due.UnixNano()), now)
		if err != nil && !errors.Is(err, store.ErrStale) {
			return err
		}
	}
	return nil
}
