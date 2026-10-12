package mcpbridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
)

type transferRecord struct {
	ID, RequestDigest, WatchID, Manifest, Revision, ReturnOf, TestKey string
	Generation                                                        int64
	Destinations                                                      map[string]string
	Review                                                            map[string]any
	Source, Target                                                    mcpcontract.Target
}

var transferID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (b *Bridge) transferPath(id string) (string, error) {
	if !transferID.MatchString(id) {
		return "", mcpclient.Fail("invalid_transfer", "use the saved transfer ID")
	}
	return filepath.Join(b.journal, "transfers", id+".json"), nil
}
func (b *Bridge) loadTransfer(id string) (transferRecord, error) {
	var t transferRecord
	path, err := b.transferPath(id)
	if err != nil {
		return t, err
	}
	if err = mcpconfig.ReadPrivateJSON(path, &t, true); err != nil {
		return t, err
	}
	if t.ID != id || t.Source.Connection == t.Target.Connection || b.backends[t.Source.Connection] == nil || b.backends[t.Target.Connection] == nil || t.Source.Instance == "" || t.Target.Instance == "" {
		return t, mcpconfig.ErrConfig
	}
	return t, nil
}
func (b *Bridge) saveTransfer(t transferRecord) error {
	path, err := b.transferPath(t.ID)
	if err != nil {
		return err
	}
	return mcpconfig.AtomicJSON(path, t)
}
func digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func dataCall(ctx context.Context, invoke call, name string, args map[string]any) (map[string]any, error) {
	view, err := invoke(ctx, name, args)
	if err != nil {
		return nil, err
	}
	data, ok := view.Data.(map[string]any)
	if !ok {
		return nil, mcpclient.Fail("invalid_response", "Ding returned an incompatible transfer result")
	}
	return data, nil
}
func step(ctx context.Context, invoke call, id, action string, input any) (map[string]any, error) {
	return dataCall(ctx, invoke, "ding_bridge_transfer_step", map[string]any{"transfer_id": id, "action": action, "input": input})
}
func requireMove(ctx context.Context, invoke call) error {
	data, err := dataCall(ctx, invoke, "ding_get_capabilities", map[string]any{})
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(data["effectiveScopes"])
	var scopes []string
	_ = json.Unmarshal(raw, &scopes)
	if !slices.Contains(scopes, "move") || data["transferTransport"] != true {
		return mcpclient.Fail("move_consent_required", "approve move, preview and manage for the native Ding bridge on both connections")
	}
	return nil
}
func transferView(t transferRecord, data map[string]any) mcpcontract.View {
	data["id"], data["source"], data["target"] = t.ID, t.Source, t.Target
	return mcpcontract.View{View: "transfer_summary", Data: data, Query: map[string]any{}, EvidenceNotice: mcpcontract.EvidenceNotice}
}

func (b *Bridge) withTransfer(ctx context.Context, t transferRecord, fn func(call, call) (mcpcontract.View, error)) (mcpcontract.View, error) {
	return b.backends[t.Source.Connection](ctx, func(source mcpcontract.Target, sourceCall call) (mcpcontract.View, error) {
		source.Connection = t.Source.Connection
		if source != t.Source {
			return mcpcontract.View{}, mcpclient.Fail("target_changed", "the source no longer matches the saved transfer")
		}
		return b.backends[t.Target.Connection](ctx, func(target mcpcontract.Target, targetCall call) (mcpcontract.View, error) {
			target.Connection = t.Target.Connection
			if target != t.Target {
				return mcpcontract.View{}, mcpclient.Fail("target_changed", "the destination no longer matches the saved transfer")
			}
			return fn(sourceCall, targetCall)
		})
	})
}
