package watchrun

import (
	"context"
	"errors"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
)

// WatchDetail reads the record, its evidence summaries, and the current hold in
// one snapshot. An integration supplies the grant authenticated by its adapter.
func (a *App) WatchDetail(ctx context.Context, id, after string, before int64, grant *store.IntegrationGrant) (store.WatchSummary, error) {
	var out store.WatchSummary
	err := a.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.InspectSummary(id, after, before)
		if err != nil {
			return err
		}
		out.Lifecycle, err = a.lifecycleGuidance(ctx, tx, out.Watch, grant)
		return err
	})
	return out, err
}

func (a *App) lifecycleGuidance(ctx context.Context, tx *store.Tx, record store.WatchRecord, grant *store.IntegrationGrant) (*store.LifecycleGuidance, error) {
	out := &store.LifecycleGuidance{Actions: []string{}}
	held, err := tx.HandoffHold(record.Plan.Definition.Metadata.ID)
	if err != nil {
		return nil, err
	}
	if held != "" {
		out.TransferID, out.Reason = held, "transfer_held"
		return out, nil
	}
	if a.isClosing() {
		out.Reason = "shutting_down"
		return out, nil
	}
	if access.Require(ctx, access.Manage) != nil || (grant != nil && !grant.Allows("manage", a.Now())) {
		out.Reason = "manage_unavailable"
		return out, nil
	}
	bundle := plan.Bundle{Watches: []plan.Compiled{record.Plan}}
	if err := a.authorizeBundle(ctx, tx, bundle); err != nil {
		if !errors.Is(err, access.ErrDenied) {
			return nil, err
		}
		out.Reason = "manage_unavailable"
		return out, nil
	}
	switch record.Status {
	case "running", "paused", "deleted":
	default:
		out.Reason = "unknown_status"
		return out, nil
	}
	for _, action := range []string{"pause", "resume", "delete", "purge"} {
		target, err := record.LifecycleTarget(action)
		if err != nil || target == record.Status {
			continue
		}
		// The MCP tool catalog exposes delete, but has no purge tool.
		if grant != nil && action == "purge" {
			continue
		}
		if grant != nil && action == "resume" && commandAccess(*grant, bundle) != nil {
			out.Reason = "resume_requires_grant"
			continue
		}
		out.Actions = append(out.Actions, action)
	}
	return out, nil
}
