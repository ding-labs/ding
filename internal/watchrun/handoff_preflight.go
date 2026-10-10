package watchrun

import (
	"context"
	"github.com/ding-labs/ding/internal/store"
)

type HandoffPreflight struct {
	Instance     string            `json:"instance"`
	Manifest     string            `json:"manifest"`
	Watch        store.WatchRecord `json:"watch"`
	Destinations map[string]string `json:"destinations"`
	Ready        bool              `json:"ready"`
	Reason       string            `json:"reason,omitempty"`
}

func (a *App) PreflightHandoff(ctx context.Context, id string) (HandoffPreflight, error) {
	var out HandoffPreflight
	err := a.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		out.Watch, err = tx.Watch(id)
		if err != nil {
			return err
		}
		out.Instance, err = tx.Identity()
		if err != nil {
			return err
		}
		out.Manifest, err = exportWatch(tx, id)
		if err != nil {
			return err
		}
		out.Destinations = map[string]string{}
		for _, d := range out.Watch.Plan.Definition.Spec.Destinations {
			value, err := tx.Destination(d.Ref, "")
			if err != nil {
				return err
			}
			out.Destinations[d.Ref] = value.Revision
		}
		if err := checkHandoffHold(tx, id, noMutation{}); err != nil {
			out.Reason = err.Error()
			return nil
		}
		if err := healthyForHandoff(tx, out.Watch, a.Now()); err != nil {
			out.Reason = err.Error()
			return nil
		}
		out.Ready = true
		return nil
	})
	return out, err
}
