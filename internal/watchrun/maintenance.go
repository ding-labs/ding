package watchrun

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

// Maintain preserves active state and outbox evidence while expiring ordinary
// history. Open incidents are retained even beyond idleTTL; quota pressure stays
// visible instead of silently deleting them.
func (a *App) Maintain(ctx context.Context, now time.Time) error {
	return a.Store.Update(ctx, func(tx *store.Tx) error {
		records, err := tx.Watches()
		if err != nil {
			return err
		}
		var pins []int64
		var eventPins []string
		for _, r := range records {
			id := r.Plan.Definition.Metadata.ID
			states, err := tx.Entities(id)
			if err != nil {
				return err
			}
			idle, _ := time.ParseDuration(r.Plan.Definition.Spec.Limits.IdleTTL)
			interval, _ := time.ParseDuration(r.Plan.Definition.Spec.Policy.Interval)
			var window time.Duration
			if r.Plan.Definition.Spec.Condition.Numeric != "" {
				expression, err := condition.ParseExpression(r.Plan.Definition.Spec.Condition.Numeric)
				if err != nil {
					return err
				}
				for _, leaf := range expression.Windows() {
					if leaf.Window > window {
						window = leaf.Window
					}
				}
			}
			for key, data := range states {
				var state condition.State
				if err := json.Unmarshal(data, &state); err != nil {
					return err
				}
				for id, expiry := range state.Seen {
					if !now.Before(expiry) {
						delete(state.Seen, id)
					}
				}
				_, timerErr := tx.Timer(id, key, "missing")
				expire := r.Status == "deleted" || (len(state.Baseline) == 0 && len(state.Seen) == 0 && !state.Open && !state.SourceUnhealthy && timerErr == store.ErrNotFound && !state.LastAt.IsZero() && !now.Before(state.LastAt.Add(idle)) && !now.Before(state.LastFired.Add(interval)) && !now.Before(state.LastAt.Add(window)))
				if expire {
					if err := tx.DeleteEntity(id, key); err != nil {
						return err
					}
					eventID, _ := watch.Revision(struct {
						Watch, Entity string
						Sequence      int64
						Generation    int64
					}{id, key, state.LastSequence, r.Generation})
					if _, err := tx.AppendEvent(watch.Event{ID: eventID, WatchID: id, Revision: r.Plan.Revision, Entity: key, Type: "entity_expired", At: now, Message: "idle state expired"}); err != nil {
						return err
					}
					continue
				}
				if timerErr != nil && timerErr != store.ErrNotFound {
					return timerErr
				}
				retained := state.Samples[:0]
				for _, sample := range state.Samples {
					if sample.At.After(now.Add(-window)) {
						retained = append(retained, sample)
						pins = append(pins, sample.Sequence)
					}
				}
				state.Samples = retained
				if !state.OverflowUntil.IsZero() && !now.Before(state.OverflowUntil) {
					state.OverflowUntil = time.Time{}
				}
				eventPins = append(eventPins, state.IncidentEvent, state.HealthEvent)
				pins = append(pins, state.Evidence...)
				pins = append(pins, state.LastSequence, state.FreshSequence, state.BaselineSequence)
				if err := tx.SaveEntity(id, key, r.Plan.Revision, state, state.LastAt); err != nil {
					return err
				}
			}
		}
		return tx.Retain(now.Add(-a.Limits.Retention), now, pins, eventPins)
	})
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
