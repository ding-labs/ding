package mcpbridge

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
)

func (b *Bridge) transfer(ctx context.Context, name string, args map[string]any) (view mcpcontract.View, err error) {
	defer func() {
		var safe *mcpclient.Error
		if err != nil && !errors.As(err, &safe) {
			err = mcpclient.Fail("transfer_journal_unavailable", "cannot read or save the private transfer journal; retain it for recovery")
		}
	}()
	unlock, err := mcpconfig.Lock(filepath.Join(b.journal, "transfers", "coordinator.lock"))
	if err != nil {
		return view, mcpclient.Fail("transfer_busy", "another transfer action is active; retry the same action")
	}
	defer unlock()
	if name == "ding_prepare_transfer" {
		return b.prepareTransfer(ctx, args)
	}
	id, _ := args["transfer_id"].(string)
	t, err := b.loadTransfer(id)
	if err != nil {
		return view, err
	}
	return b.withTransfer(ctx, t, func(source, target call) (mcpcontract.View, error) {
		if name == "ding_get_transfer_status" {
			return transferStatus(ctx, t, source, target)
		}
		if err := requireMove(ctx, source); err != nil {
			return view, err
		}
		if err := requireMove(ctx, target); err != nil {
			return view, err
		}
		switch name {
		case "ding_test_transfer":
			key, _ := args["operation_key"].(string)
			if len(key) < 16 || len(key) > 128 {
				return view, mcpclient.Fail("invalid_operation", "provide a stable test operation key")
			}
			if err := bind(b.journal, key, name, args, t.Target, true); err != nil {
				return view, err
			}
			t.TestKey = key
			if err := b.saveTransfer(t); err != nil {
				return view, err
			}
			out, err := step(ctx, target, t.ID, "test", map[string]any{"operationKey": key})
			if err != nil {
				return view, err
			}
			return transferView(t, map[string]any{"phase": "tested", "outcome": out["outcome"], "note": "Confirm that you received this labeled test before authorizing finish. Testing does not pause the source."}), nil
		case "ding_finish_transfer":
			if args["delivery_confirmed"] != true {
				return view, mcpclient.Fail("delivery_confirmation_required", "the user must confirm the labeled target delivery and authorize the handoff")
			}
			return finishTransfer(ctx, t, source, target)
		case "ding_cancel_transfer":
			if args["cancel_confirmed"] != true {
				return view, mcpclient.Fail("transfer_review_required", "confirm cancellation of this prepared transfer")
			}
			canceled, err := step(ctx, target, t.ID, "cancel", map[string]any{})
			if err != nil {
				return view, mcpclient.Fail("transfer_unresolved", "target inactivity is not confirmed; keep the source held and inspect transfer "+t.ID)
			}
			_, err = dataCall(ctx, source, "ding_get_transfer", map[string]any{"transfer_id": t.ID})
			if err != nil && !notFound(err) {
				return view, err
			}
			if err == nil {
				if _, err = step(ctx, source, t.ID, "release", canceled); err != nil {
					return view, err
				}
			}
			return transferView(t, map[string]any{"phase": "canceled", "note": "The never-activated target is canceled. A paused source remains paused; inspect it before explicitly resuming."}), nil
		}
		return view, mcpclient.Fail("invalid_tool", "unsupported transfer action")
	})
}

func notFound(err error) bool {
	var e *mcpclient.Error
	return errors.As(err, &e) && (e.Code == "not_found" || e.Code == "remote_error" && strings.HasPrefix(e.Message, "not_found:"))
}

func finishTransfer(ctx context.Context, t transferRecord, source, target call) (mcpcontract.View, error) {
	view := mcpcontract.View{}
	prepared, err := dataCall(ctx, target, "ding_get_transfer", map[string]any{"transfer_id": t.ID})
	if err != nil {
		return view, err
	}
	if prepared["phase"] != "prepared" && prepared["phase"] != "active" {
		return view, mcpclient.Fail("transfer_conflict", "target is not prepared or active; inspect this transfer")
	}
	if prepared["instance"] != t.Target.Instance || prepared["peer"] != t.Source.Instance || prepared["watchId"] != t.WatchID {
		return view, mcpclient.Fail("target_changed", "target receipt does not match the saved transfer")
	}
	if prepared["phase"] == "prepared" {
		if t.TestKey == "" {
			return view, mcpclient.Fail("test_required", "run a labeled target test before finishing")
		}
		tested, err := step(ctx, target, t.ID, "test", map[string]any{"operationKey": t.TestKey})
		if err != nil {
			return view, err
		}
		if tested["outcome"] != "accepted" {
			return view, mcpclient.Fail("test_required", "target delivery is not confirmed; source was not paused")
		}
		proof, err := step(ctx, source, t.ID, "pause", map[string]any{"id": t.ID, "peer": t.Target.Instance, "watchId": t.WatchID, "revision": t.Revision, "generation": t.Generation, "destinations": t.Destinations})
		if err != nil {
			return view, mcpclient.Fail("transfer_unresolved", "source pause is unresolved; inspect transfer "+t.ID+"; target was not activated by this attempt")
		}
		if _, err = step(ctx, target, t.ID, "activate", proof); err != nil {
			return view, mcpclient.Fail("transfer_unresolved", "activation is unresolved; keep the source held; inspect transfer "+t.ID)
		}
	}
	return transferStatus(ctx, t, source, target)
}

func transferStatus(ctx context.Context, t transferRecord, source, target call) (mcpcontract.View, error) {
	read := func(invoke call) map[string]any {
		h, err := dataCall(ctx, invoke, "ding_get_transfer", map[string]any{"transfer_id": t.ID})
		if notFound(err) {
			return map[string]any{"phase": "not_started"}
		}
		if err != nil {
			return map[string]any{"phase": "unknown", "error": mcpclient.SafeMessage(err)}
		}
		return map[string]any{"phase": h["phase"], "held": h["held"], "watchId": h["watchId"], "revision": h["revision"], "generation": h["generation"]}
	}
	s, d := read(source), read(target)
	out := map[string]any{"sourceReceipt": s, "targetReceipt": d, "phase": d["phase"], "note": "An unreachable peer never proves it stopped. Keep the source held after an uncertain activation."}
	if d["phase"] == "active" {
		watch, err := dataCall(ctx, target, "ding_get_watch", map[string]any{"watch_id": t.WatchID})
		w, _ := watch["watch"].(map[string]any)
		at, _ := w["lastInputAt"].(string)
		confirmed := err == nil && at != "" && !strings.HasPrefix(at, "0001-")
		out["firstObservationConfirmed"] = confirmed
		if confirmed {
			out["firstObservation"] = at
			out["note"] = "Target has observed data. Source history remains on its original instance and source execution stays held paused."
		} else {
			out["phase"] = "active_waiting_observation"
			out["note"] = "Target activation committed; first observation is not confirmed. Inspect this same transfer again; keep source held paused."
		}
	}
	return transferView(t, out), nil
}
