package cloudlink

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

type Transfer struct {
	ID           string                       `json:"id"`
	URL          string                       `json:"url"`
	Workspace    string                       `json:"workspace"`
	Source       watchrun.HandoffPreflight    `json:"source"`
	TargetReview *watchrun.ApplyPreconditions `json:"targetReview"`
	TestKey      string                       `json:"testKey"`
}

var transferID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func TransferPath(dir, id string) (string, error) {
	if !transferID.MatchString(id) {
		return "", fmt.Errorf("invalid transfer ID")
	}
	return filepath.Join(dir, "transfers", id+".json"), nil
}
func SaveTransfer(dir string, t Transfer) error {
	path, err := TransferPath(dir, t.ID)
	if err != nil {
		return err
	}
	return install.AtomicJSON(path, t)
}
func LoadTransfer(dir, id string) (Transfer, error) {
	var t Transfer
	path, err := TransferPath(dir, id)
	if err != nil {
		return t, err
	}
	err = mcpconfig.ReadPrivateJSON(path, &t, true)
	if err == nil && t.ID != id {
		err = fmt.Errorf("transfer identity mismatch")
	}
	return t, err
}
func (t Transfer) Target(c Connection) (control.Client, error) {
	if c.URL != t.URL || c.Workspace != t.Workspace {
		return control.Client{}, fmt.Errorf("sign in to the same cloud workspace that owns this transfer")
	}
	return c.Client()
}

func (t Transfer) Prepare(ctx context.Context, target control.Client) (store.Handoff, error) {
	return Call[store.Handoff](ctx, target, "POST", "/v1/handoffs/prepare", watchrun.HandoffPrepare{ID: t.ID, Peer: t.Source.Instance, Manifest: t.Source.Manifest, Review: t.TargetReview})
}
func (t Transfer) Test(ctx context.Context, target control.Client) (struct {
	Outcome string `json:"outcome"`
}, error) {
	return Call[struct {
		Outcome string `json:"outcome"`
	}](ctx, target, "POST", "/v1/handoffs/"+t.ID+"/test", map[string]string{"operationKey": t.TestKey})
}
func (t Transfer) Finish(ctx context.Context, source, target control.Client) (store.Handoff, error) {
	prepared, err := Call[store.Handoff](ctx, target, "GET", "/v1/handoffs/"+t.ID, nil)
	if err != nil {
		return prepared, err
	}
	if prepared.Phase != "prepared" && prepared.Phase != "active" {
		return prepared, fmt.Errorf("target is not prepared or active")
	}
	proof, err := Call[store.Handoff](ctx, source, "POST", "/v1/handoffs/pause", watchrun.HandoffPause{ID: t.ID, Peer: prepared.Instance, WatchID: t.Source.Watch.Plan.Definition.Metadata.ID, Revision: t.Source.Watch.Plan.Revision, Generation: t.Source.Watch.Generation, Destinations: t.Source.Destinations})
	if err != nil {
		return proof, fmt.Errorf("source pause not confirmed; target was not activated; inspect the same transfer: %w", err)
	}
	active, err := Call[store.Handoff](ctx, target, "POST", "/v1/handoffs/"+t.ID+"/activate", proof)
	if err != nil {
		return active, fmt.Errorf("activation unresolved; source remains held and paused; inspect transfer %s, never resume automatically: %w", t.ID, err)
	}
	return active, nil
}
func (t Transfer) Cancel(ctx context.Context, source, target control.Client) error {
	canceled, err := Call[store.Handoff](ctx, target, "POST", "/v1/handoffs/"+t.ID+"/cancel", struct{}{})
	if err != nil {
		return fmt.Errorf("target inactivity not confirmed; keep source paused: %w", err)
	}
	_, err = Call[store.Handoff](ctx, source, "GET", "/v1/handoffs/"+t.ID, nil)
	if api, ok := err.(*control.APIError); ok && api.Code == "not_found" {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = Call[store.Handoff](ctx, source, "POST", "/v1/handoffs/"+t.ID+"/release", canceled)
	return err
}
