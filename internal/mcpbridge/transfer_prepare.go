package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpcontract"
)

func (b *Bridge) prepareTransfer(ctx context.Context, args map[string]any) (mcpcontract.View, error) {
	sourceID, _ := args["source_connection"].(string)
	targetID, _ := args["target_connection"].(string)
	if args["upload_confirmed"] != true || sourceID == targetID || b.backends[sourceID] == nil || b.backends[targetID] == nil {
		return mcpcontract.View{}, mcpclient.Fail("transfer_review_required", "review the exact source definition and approve its upload to the other connection")
	}
	key, _ := args["operation_key"].(string)
	if len(key) < 16 || len(key) > 128 {
		return mcpcontract.View{}, mcpclient.Fail("invalid_operation", "provide a stable operation key")
	}
	id := digest(key)
	old, err := b.loadTransfer(id)
	if err == nil {
		if old.RequestDigest != digest(args) {
			return mcpcontract.View{}, mcpclient.Fail("operation_conflict", "this transfer key belongs to a different reviewed request")
		}
		return b.withTransfer(ctx, old, func(sourceCall call, targetCall call) (mcpcontract.View, error) {
			if err := requireMove(ctx, sourceCall); err != nil {
				return mcpcontract.View{}, err
			}
			if err := requireMove(ctx, targetCall); err != nil {
				return mcpcontract.View{}, err
			}
			return prepareSaved(ctx, old, targetCall)
		})
	}
	if !errors.Is(err, os.ErrNotExist) {
		return mcpcontract.View{}, err
	}
	// Read and reject ineligible local data before opening the remote profile.
	return b.backends[sourceID](ctx, func(source mcpcontract.Target, sourceCall call) (mcpcontract.View, error) {
		if err := requireMove(ctx, sourceCall); err != nil {
			return mcpcontract.View{}, err
		}
		preflight, err := dataCall(ctx, sourceCall, "ding_get_transfer_preflight", map[string]any{"watch_id": args["watch_id"]})
		if err != nil {
			return mcpcontract.View{}, err
		}
		var checked struct {
			Instance, Manifest, Reason string
			Ready                      bool
			Destinations               map[string]string
			Watch                      struct {
				Generation int64
				Plan       struct{ Revision string }
			}
		}
		raw, _ := json.Marshal(preflight)
		if json.Unmarshal(raw, &checked) != nil || !checked.Ready {
			return mcpcontract.View{}, mcpclient.Fail("transfer_ineligible", "the source is not eligible; inspect its local preflight before connecting or uploading")
		}
		if checked.Instance != source.Instance || args["expected_revision"] != checked.Watch.Plan.Revision || digest(args["expected_generation"]) != digest(checked.Watch.Generation) || digest(args["expected_destinations"]) != digest(checked.Destinations) {
			return mcpcontract.View{}, mcpclient.Fail("revision_conflict", "source or destinations changed since review; inspect the preflight again")
		}
		source.Connection = sourceID
		if err := bind(b.journal, key, "ding_prepare_transfer", args, source, true); err != nil {
			return mcpcontract.View{}, err
		}
		return b.backends[targetID](ctx, func(target mcpcontract.Target, targetCall call) (mcpcontract.View, error) {
			target.Connection = targetID
			if source.Instance == target.Instance {
				return mcpcontract.View{}, mcpclient.Fail("invalid_target", "source and destination must be different instances")
			}
			if err := requireMove(ctx, targetCall); err != nil {
				return mcpcontract.View{}, err
			}
			preview, err := dataCall(ctx, targetCall, "ding_preview_changes", map[string]any{"manifest": checked.Manifest})
			if err != nil {
				return mcpcontract.View{}, err
			}
			p, _ := preview["preview"].(map[string]any)
			changes, _ := p["changes"].(map[string]any)
			review, _ := changes["review"].(map[string]any)
			if preview["valid"] != true || review == nil {
				return mcpcontract.View{}, mcpclient.Fail("transfer_ineligible", "the destination rejected this definition; inspect its capabilities and credential references")
			}
			credentials, _ := changes["credentials"].([]any)
			for _, item := range credentials {
				credential, ok := item.(map[string]any)
				if !ok || credential["present"] != true {
					return mcpcontract.View{}, mcpclient.Fail("credential_required", "rebind required credential references in protected setup before preparing")
				}
			}
			existing, err := dataCall(ctx, targetCall, "ding_list_transfers", map[string]any{})
			if err != nil {
				return mcpcontract.View{}, err
			}
			returnOf := ""
			records, _ := existing["transfers"].([]any)
			for _, item := range records {
				h, ok := item.(map[string]any)
				if ok && h["role"] == "source" && h["phase"] == "paused" && h["held"] == true && h["watchId"] == args["watch_id"] && h["peer"] == source.Instance {
					returnOf, _ = h["id"].(string)
				}
			}
			watchID, _ := args["watch_id"].(string)
			t := transferRecord{ID: id, RequestDigest: digest(args), WatchID: watchID, Manifest: checked.Manifest, Revision: checked.Watch.Plan.Revision, Generation: checked.Watch.Generation, Destinations: checked.Destinations, Review: review, ReturnOf: returnOf, Source: source, Target: target}
			if err := b.saveTransfer(t); err != nil {
				return mcpcontract.View{}, err
			}
			return prepareSaved(ctx, t, targetCall)
		})
	})
}

func prepareSaved(ctx context.Context, t transferRecord, targetCall call) (mcpcontract.View, error) {
	out, err := step(ctx, targetCall, t.ID, "prepare", map[string]any{"id": t.ID, "peer": t.Source.Instance, "manifest": t.Manifest, "review": t.Review, "returnOf": t.ReturnOf})
	if err != nil {
		return mcpcontract.View{}, mcpclient.Fail("transfer_unresolved", "preparation is unresolved; inspect saved transfer "+t.ID+" before retrying; the source was not paused by preparation")
	}
	return transferView(t, map[string]any{"phase": out["phase"], "note": "Preparation does not pause the source. A prepared target stays held paused until a tested, confirmed handoff."}), nil
}
