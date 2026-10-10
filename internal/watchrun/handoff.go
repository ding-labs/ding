package watchrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

type HandoffPrepare struct {
	ID       string              `json:"id"`
	Peer     string              `json:"peer"`
	Manifest string              `json:"manifest"`
	Review   *ApplyPreconditions `json:"review"`
}
type HandoffPause struct {
	ID           string            `json:"id"`
	Peer         string            `json:"peer"`
	WatchID      string            `json:"watchId"`
	Revision     string            `json:"revision"`
	Generation   int64             `json:"generation"`
	Destinations map[string]string `json:"destinations"`
}

type handoffMutation struct {
	h      store.Handoff
	action string
	check  func(*store.Tx) error
}

func (m *handoffMutation) handoffID() string { return m.h.ID }
func (m *handoffMutation) begin(tx *store.Tx, result any, now time.Time) (bool, error) {
	old, err := tx.Handoff(m.h.ID)
	if err == nil {
		if old.Digest != m.h.Digest || old.Role != m.h.Role || old.Peer != m.h.Peer || old.WatchID != m.h.WatchID {
			return false, store.ErrConflict
		}
		if m.action == "prepare" || m.action == "pause" {
			return true, nil
		}
		return false, store.ErrConflict
	}
	if !errors.Is(err, store.ErrNotFound) {
		return false, err
	}
	if m.check != nil {
		return false, m.check(tx)
	}
	return false, nil
}
func (m *handoffMutation) finish(tx *store.Tx, _ any, now time.Time) error {
	r, err := tx.Watch(m.h.WatchID)
	if err != nil {
		return err
	}
	m.h.Revision, m.h.Generation, m.h.At = r.Plan.Revision, r.Generation, now
	m.h.Destinations = map[string]string{}
	for _, target := range r.Plan.Definition.Spec.Destinations {
		d, err := tx.Destination(target.Ref, "")
		if err != nil {
			return err
		}
		m.h.Destinations[target.Ref] = d.Revision
	}
	return tx.SaveHandoff(m.h)
}
func (a *App) Handoff(ctx context.Context, id string) (store.Handoff, error) {
	var h store.Handoff
	err := a.Store.View(ctx, func(tx *store.Tx) error { var e error; h, e = tx.Handoff(id); return e })
	return h, err
}

// PrepareHandoff creates the selected destination watch paused in the same
// transaction as its ownership hold. No acquisition can occur between steps.
func (a *App) PrepareHandoff(ctx context.Context, r HandoffPrepare) (store.Handoff, error) {
	if len(r.Manifest) > 64<<10 || r.Review == nil || r.Peer == "" {
		return store.Handoff{}, fmt.Errorf("bounded manifest, exact review and peer required")
	}
	b, err := plan.Parse([]byte(r.Manifest))
	if err != nil {
		return store.Handoff{}, err
	}
	if len(b.Watches) != 1 {
		return store.Handoff{}, fmt.Errorf("transfer exactly one watch")
	}
	digest, err := watch.Revision(struct{ Manifest, Peer string }{r.Manifest, r.Peer})
	if err != nil {
		return store.Handoff{}, err
	}
	m := &handoffMutation{h: store.Handoff{ID: r.ID, Peer: r.Peer, WatchID: b.Watches[0].Definition.Metadata.ID, Digest: digest, Role: "target", Phase: "prepared", Held: true}, action: "prepare"}
	_, err = a.apply(ctx, ApplyRequest{Manifest: r.Manifest, Review: r.Review, startPaused: true}, m)
	if err != nil {
		return store.Handoff{}, err
	}
	return a.Handoff(ctx, r.ID)
}

func healthyForHandoff(tx *store.Tx, r store.WatchRecord, now time.Time) error {
	if r.Status != "running" || r.LastError != "" || r.LastInputAt.IsZero() {
		return fmt.Errorf("move requires a running watch with a successful observation")
	}
	interval, _ := time.ParseDuration(r.Plan.Definition.Spec.Source.Every)
	timeout, _ := time.ParseDuration(r.Plan.Definition.Spec.Source.Timeout)
	if now.Sub(r.LastInputAt) > 2*interval+timeout {
		return fmt.Errorf("last observation is overdue; repair the source before moving")
	}
	pending, err := tx.PendingForWatch(r.Plan.Definition.Metadata.ID)
	if err != nil {
		return err
	}
	if pending != 0 {
		return fmt.Errorf("wait for pending and in-flight deliveries before moving")
	}
	entities, err := tx.Entities(r.Plan.Definition.Metadata.ID)
	if err != nil {
		return err
	}
	for _, raw := range entities {
		var s condition.State
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		if s.Open || s.SourceUnhealthy || s.Matches > 0 || s.Recoveries > 0 || len(s.Samples) > 0 || len(s.Seen) > 0 || len(s.Baseline) > 0 {
			return fmt.Errorf("move requires healthy idle evaluation state; active incidents and stateful windows cannot move yet")
		}
	}
	return nil
}

func (a *App) PauseForHandoff(ctx context.Context, r HandoffPause) (store.Handoff, error) {
	if r.Peer == "" || r.Revision == "" || r.Generation < 1 {
		return store.Handoff{}, fmt.Errorf("peer and exact watch generation required")
	}
	digest, err := watch.Revision(r)
	if err != nil {
		return store.Handoff{}, err
	}
	m := &handoffMutation{h: store.Handoff{ID: r.ID, Peer: r.Peer, WatchID: r.WatchID, Digest: digest, Role: "source", Phase: "paused", Held: true}, action: "pause"}
	m.check = func(tx *store.Tx) error {
		record, err := tx.Watch(r.WatchID)
		if err != nil {
			return err
		}
		if err := healthyForHandoff(tx, record, a.Now()); err != nil {
			return err
		}
		if len(r.Destinations) != len(record.Plan.Definition.Spec.Destinations) {
			return store.ErrConflict
		}
		for _, target := range record.Plan.Definition.Spec.Destinations {
			d, err := tx.Destination(target.Ref, "")
			if err != nil {
				return err
			}
			if r.Destinations[target.Ref] != d.Revision {
				return store.ErrConflict
			}
		}
		return nil
	}
	_, err = a.lifecycle(ctx, r.WatchID, LifecycleRequest{Action: "pause", Expected: r.Revision, ExpectedGeneration: &r.Generation}, m)
	if err != nil {
		return store.Handoff{}, err
	}
	return a.Handoff(ctx, r.ID)
}
