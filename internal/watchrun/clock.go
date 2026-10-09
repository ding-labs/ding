package watchrun

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/replay"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func clockDiscontinuity(previous, current time.Time, elapsed time.Duration) bool {
	difference := current.Sub(previous) - elapsed
	return difference > 2*time.Second || difference < -2*time.Second
}

// Runtime wall-time jumps are compared with Go's monotonic clock. Restart cannot
// reconstruct a monotonic clock across downtime; normal catch-up rules apply.
func (a *App) clockShift(ctx context.Context, now time.Time) error {
	generations := map[string]int64{}
	err := a.Store.Update(ctx, func(tx *store.Tx) error {
		records, err := tx.Watches()
		if err != nil {
			return err
		}
		for _, record := range records {
			if record.Status != "running" {
				continue
			}
			id := record.Plan.Definition.Metadata.ID
			record.Generation++
			record.NextAt = now
			if err := tx.SaveWatch(record, now); err != nil {
				return err
			}
			if err := tx.DeleteTimers(id); err != nil {
				return err
			}
			states, err := tx.Entities(id)
			if err != nil {
				return err
			}
			e, err := condition.New(record.Plan.Definition, record.Plan.Revision)
			if err != nil {
				return err
			}
			for key, data := range states {
				var prior condition.State
				if err := json.Unmarshal(data, &prior); err != nil {
					return err
				}
				o := watch.Observation{InputID: fmt.Sprintf("clock:%d:%s", record.Generation, key), AcceptedAt: now, Health: "clock", Detail: "clock_discontinuity"}
				o.Sequence, err = tx.AppendObservation(id, record.Plan.Revision, record.Generation, o)
				if err != nil {
					return err
				}
				result, err := e.Evaluate(prior, o, now)
				if err != nil {
					return err
				}
				if err := tx.SaveEntity(id, key, record.Plan.Revision, result.State, now); err != nil {
					return err
				}
				if record.Plan.Definition.Spec.Condition.MissingFor != "" {
					if err := tx.SaveTimer(store.Timer{WatchID: id, Entity: key, Kind: "missing", Generation: record.Generation, Due: result.State.MissingAt}); err != nil {
						return err
					}
				}
				for _, event := range result.Events {
					if err := a.appendEvaluated(tx, record, event, now, replay.Checkpoint{Prior: prior, Input: o}); err != nil {
						return err
					}
				}
			}
			if len(states) == 0 {
				if err := armInitial(tx, record, now); err != nil {
					return err
				}
			}
			if err := lifecycleEvent(tx, record, "clock_discontinuity", "wall clock diverged from monotonic time; temporal evidence rearmed", now); err != nil {
				return err
			}
			generations[id] = record.Generation
		}
		return nil
	})
	if err == nil {
		for id, generation := range generations {
			a.cancelBefore(id, generation)
		}
	}
	return err
}
