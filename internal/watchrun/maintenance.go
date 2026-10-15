package watchrun

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/runtimeperf"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

// Maintain uses short write batches. Persistent entity references are updated
// with state, so ingestion can advance safely between every cleanup transaction.
func (a *App) Maintain(ctx context.Context, now time.Time) error {
	ctx = runtimeperf.WithMaintenance(ctx)
	defer runtimeperf.Since(a.Store.PerfBackend(), runtimeperf.Maintenance, time.Now())
	const batch = 64
	pass := rand.Text()
	for {
		done := false
		err := a.Store.Update(ctx, func(tx *store.Tx) error {
			entities, end, err := tx.NextMaintenanceEntities(batch)
			if err != nil {
				return err
			}
			done = end
			policies := map[string]maintenancePolicy{}
			for _, entity := range entities {
				policy, ok := policies[entity.WatchID]
				if !ok {
					record, err := tx.Watch(entity.WatchID)
					if err != nil {
						return err
					}
					policy, err = newMaintenancePolicy(record)
					if err != nil {
						return err
					}
					policies[entity.WatchID] = policy
				}
				if err = maintainEntity(tx, policy, entity.Key, entity.State, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if done {
			break
		}
		runtime.Gosched()
	}
	for {
		var changed int64
		err := a.Store.Update(ctx, func(tx *store.Tx) error {
			var err error
			changed, err = tx.RetainBatch(now.Add(-a.Limits.Retention), now, batch, pass)
			return err
		})
		if err != nil {
			return err
		}
		if changed == 0 {
			break
		}
		runtime.Gosched()
	}
	var records []store.WatchRecord
	if err := a.Store.View(ctx, func(tx *store.Tx) error { var err error; records, err = tx.Watches(); return err }); err != nil {
		return err
	}
	for _, r := range records {
		if c := r.Plan.Definition.Spec.Source.Logs; c != nil {
			retention, _ := time.ParseDuration(c.Retention)
			if retention == 0 {
				retention = time.Hour
			}
			for {
				var changed int64
				err := a.Store.Update(ctx, func(tx *store.Tx) error {
					var err error
					changed, err = tx.RetainLogSummariesBatch(r.Plan.Definition.Metadata.ID, now.Add(-retention), batch, pass)
					return err
				})
				if err != nil {
					return err
				}
				if changed == 0 {
					break
				}
				runtime.Gosched()
			}
		}
	}
	return nil
}

type maintenancePolicy struct {
	record                 store.WatchRecord
	idle, interval, window time.Duration
}

func newMaintenancePolicy(r store.WatchRecord) (maintenancePolicy, error) {
	p := maintenancePolicy{record: r}
	p.idle, _ = time.ParseDuration(r.Plan.Definition.Spec.Limits.IdleTTL)
	p.interval, _ = time.ParseDuration(r.Plan.Definition.Spec.Policy.Interval)
	if r.Plan.Definition.Spec.Condition.Numeric != "" {
		expression, err := condition.ParseExpression(r.Plan.Definition.Spec.Condition.Numeric)
		if err != nil {
			return p, err
		}
		for _, leaf := range expression.Windows() {
			if leaf.Window > p.window {
				p.window = leaf.Window
			}
		}
	}
	return p, nil
}

func maintainEntity(tx *store.Tx, p maintenancePolicy, key string, data json.RawMessage, now time.Time) error {
	r := p.record
	id := r.Plan.Definition.Metadata.ID
	var state condition.State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	for input, expiry := range state.Seen {
		if !now.Before(expiry) {
			delete(state.Seen, input)
		}
	}
	_, timerErr := tx.Timer(id, key, "missing")
	if timerErr != nil && timerErr != store.ErrNotFound {
		return timerErr
	}
	expire := r.Status == "deleted" || (len(state.Baseline) == 0 && len(state.Seen) == 0 && !state.Open && !state.SourceUnhealthy && timerErr == store.ErrNotFound && !state.LastAt.IsZero() && !now.Before(state.LastAt.Add(p.idle)) && !now.Before(state.LastFired.Add(p.interval)) && !now.Before(state.LastAt.Add(p.window)))
	if expire {
		if err := tx.DeleteEntity(id, key); err != nil {
			return err
		}
		eventID, _ := watch.Revision(struct {
			Watch, Entity        string
			Sequence, Generation int64
		}{id, key, state.LastSequence, r.Generation})
		_, err := tx.AppendEvent(watch.Event{ID: eventID, WatchID: id, Revision: r.Plan.Revision, Entity: key, Type: "entity_expired", At: now, Message: "idle state expired"})
		return err
	}
	retained := state.Samples[:0]
	for _, sample := range state.Samples {
		if sample.At.After(now.Add(-p.window)) {
			retained = append(retained, sample)
		}
	}
	state.Samples = retained
	if !state.OverflowUntil.IsZero() && !now.Before(state.OverflowUntil) {
		state.OverflowUntil = time.Time{}
	}
	return tx.SaveEntity(id, key, r.Plan.Revision, state, state.LastAt)
}
func (a *App) Budget(ctx context.Context) (store.Usage, error) {
	var usage store.Usage
	err := a.Store.View(ctx, func(tx *store.Tx) error { var err error; usage, err = tx.Budget(); return err })
	if err != nil {
		return usage, err
	}
	if usage.Bytes >= a.Limits.MaxBytes || usage.Pending >= a.Limits.MaxPending {
		return usage, ErrQuota
	}
	return usage, nil
}
func (a *App) validateLimits() error {
	if a.Limits.MaxWatches < 1 || a.Limits.MaxPending < 1 || a.Limits.MaxBytes < 1 || a.Limits.Retention < time.Second {
		return fmt.Errorf("runtime limits must be positive")
	}
	return nil
}
