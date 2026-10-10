package watchrun

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ding-labs/ding/internal/store"
)

// ActivateHandoff accepts the source's authenticated pause receipt. The
// coordinator obtains that receipt over the source's admin API; there is no
// cross-machine atomic transaction. It must never invent one after a timeout.
func (a *App) ActivateHandoff(ctx context.Context, id string, source store.Handoff) (store.Handoff, error) {
	var out store.Handoff
	err := a.Store.Update(ctx, func(tx *store.Tx) error {
		h, err := tx.Handoff(id)
		if err != nil {
			return err
		}
		if h.Role != "target" || source.Role != "source" || source.Phase != "paused" || !source.Held || source.ID != h.ID || source.Peer != h.Instance || source.Instance != h.Peer || source.WatchID != h.WatchID || source.Revision != h.Revision || !reflect.DeepEqual(source.Destinations, h.Destinations) {
			return fmt.Errorf("source pause receipt does not match this prepared transfer")
		}
		if h.Phase == "active" {
			out = h
			return nil
		}
		if h.Phase != "prepared" || !h.Held {
			return store.ErrConflict
		}
		r, err := tx.Watch(h.WatchID)
		if err != nil {
			return err
		}
		if r.Status != "paused" || r.Plan.Revision != h.Revision || r.Generation != h.Generation {
			return store.ErrConflict
		}
		for id, revision := range h.Destinations {
			d, err := tx.Destination(id, "")
			if err != nil {
				return err
			}
			if d.Revision != revision {
				return store.ErrConflict
			}
		}
		if a.isClosing() {
			return ErrClosing
		}
		r.Status = "running"
		r.Generation++
		r.NextAt = a.Now()
		r.LastError = ""
		if err := tx.SaveWatch(r, a.Now()); err != nil {
			return err
		}
		if err := armInitial(tx, r, a.Now()); err != nil {
			return err
		}
		if err := lifecycleEvent(tx, r, "resumed", "transfer activated with fresh evaluation state", a.Now()); err != nil {
			return err
		}
		h.Phase = "active"
		h.Held = false
		h.Generation = r.Generation
		out = h
		return tx.SaveHandoff(h)
	})
	return out, err
}

// CancelPreparedHandoff is safe only on a target that never became active.
// It permanently deletes the prepared watch; its receipt allows source release.
func (a *App) CancelPreparedHandoff(ctx context.Context, id string) (store.Handoff, error) {
	var out store.Handoff
	err := a.Store.Update(ctx, func(tx *store.Tx) error {
		h, err := tx.Handoff(id)
		if err != nil {
			return err
		}
		if h.Role != "target" {
			return store.ErrConflict
		}
		if h.Phase == "canceled" {
			out = h
			return nil
		}
		if h.Phase != "prepared" || !h.Held {
			return fmt.Errorf("target became active; perform a reverse transfer instead")
		}
		r, err := tx.Watch(h.WatchID)
		if err != nil {
			return err
		}
		if r.Status != "paused" || r.Generation != h.Generation || r.Plan.Revision != h.Revision {
			return store.ErrConflict
		}
		r.Status = "deleted"
		if h.ReturnOf != "" {
			r.Status = "paused"
		}
		r.Generation++
		if err := tx.SaveWatch(r, a.Now()); err != nil {
			return err
		}
		if err := tx.DeleteTimers(h.WatchID); err != nil {
			return err
		}
		if h.ReturnOf != "" {
			original, err := tx.Handoff(h.ReturnOf)
			if err != nil {
				return err
			}
			if original.Phase != "returned" || original.Held {
				return store.ErrConflict
			}
			original.Phase = "paused"
			original.Held = true
			original.Generation = r.Generation
			original.Revision = r.Plan.Revision
			original.Destinations = h.Destinations
			h.Held = false
			if err := tx.SaveHandoff(h); err != nil {
				return err
			}
			if err := tx.SaveHandoff(original); err != nil {
				return err
			}
		}
		h.Phase = "canceled"
		h.Held = false
		out = h
		return tx.SaveHandoff(h)
	})
	return out, err
}

// ReleaseHandoff leaves the source paused. Only a matching canceled target
// receipt permits the user's subsequent ordinary resume, never a network error.
func (a *App) ReleaseHandoff(ctx context.Context, id string, target store.Handoff) (store.Handoff, error) {
	var out store.Handoff
	err := a.Store.Update(ctx, func(tx *store.Tx) error {
		h, err := tx.Handoff(id)
		if err != nil {
			return err
		}
		if h.Role != "source" || target.Role != "target" || target.ID != id || target.Phase != "canceled" || target.Held || target.Peer != h.Instance || target.Instance != h.Peer || target.WatchID != h.WatchID || target.Revision != h.Revision {
			return fmt.Errorf("confirmed canceled target receipt required")
		}
		if h.Phase == "released" {
			out = h
			return nil
		}
		if h.Phase != "paused" {
			return store.ErrConflict
		}
		h.Held = false
		h.Phase = "released"
		out = h
		return tx.SaveHandoff(h)
	})
	return out, err
}
