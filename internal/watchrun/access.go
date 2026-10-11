package watchrun

import (
	"context"
	"reflect"
	"slices"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
)

// authorizeBundle runs in the watch transaction, including preview. Membership
// admission is held by the server while this transaction completes. Integration
// command/secret grants remain an additional, independent restriction.
func (a *App) authorizeBundle(ctx context.Context, tx *store.Tx, b plan.Bundle) error {
	p, shared := access.FromContext(ctx)
	if !shared || p.Operator {
		return nil
	}
	for _, d := range b.Destinations {
		old, err := tx.Destination(d.Definition.Metadata.ID, "")
		if (err != nil || old.Revision != d.Revision) && !p.Allows(access.Destinations) {
			return access.ErrDenied
		}
	}
	for _, w := range b.Watches {
		src := w.Definition.Spec.Source
		if src.Type == "command" && !slices.Contains(a.ApprovedCommands, w.Revision) {
			return access.ErrDenied
		}
		if src.Type != "command" && src.Type != "http" && src.Type != "push" {
			return access.ErrDenied
		}
		if !(p.Allows(access.Credentials) || (p.Kind == "integration" && (p.Role == access.Administrator || p.Role == access.Owner))) && (src.URLRef != nil || len(src.Headers) > 0 || len(src.Env) > 0) {
			old, err := tx.Watch(w.Definition.Metadata.ID)
			if err != nil {
				return access.ErrDenied
			}
			previous := old.Plan.Definition.Spec.Source
			// Cadence, timeout and projected fields do not change where the saved
			// credential is used. Endpoint, headers and environment must match.
			src.Every, src.Timeout, src.Fields, src.ObservedAtField = "", "", nil, ""
			previous.Every, previous.Timeout, previous.Fields, previous.ObservedAtField = "", "", nil, ""
			if !reflect.DeepEqual(src, previous) {
				return access.ErrDenied
			}
		}
	}
	return nil
}
