package watchrun

import (
	"errors"
	"github.com/ding-labs/ding/internal/store"
	"strings"
	"testing"
)

func TestReviewedApplyGuardsEntireBundleAndReferencedDestinations(t *testing.T) {
	a, _ := setup(t)
	original := manifest("https://example.com")
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: original}); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(original, "consecutive: 3", "consecutive: 4", 1)
	preview, err := a.Apply(ctx, ApplyRequest{Manifest: edited, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Review == nil || len(preview.DestinationChanges) != 1 || preview.Changes[0].Before == "" || preview.Changes[0].State != "reset" {
		t.Fatal(preview)
	}
	other := strings.Replace(original, "WEBHOOK_URL", "OTHER_URL", 1)
	if _, err = a.Apply(ctx, ApplyRequest{Manifest: other}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Apply(ctx, ApplyRequest{Manifest: edited, Review: preview.Review}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale destination overwritten", err)
	}
	record, err := a.Record(ctx, "api")
	if err != nil || record.Plan.Definition.Spec.Policy.Consecutive != 3 {
		t.Fatal("partial watch commit", record, err)
	}
	// Editing the draft invalidates the exact review even if all current revisions match.
	preview, err = a.Apply(ctx, ApplyRequest{Manifest: edited, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Apply(ctx, ApplyRequest{Manifest: edited + "\n", Review: preview.Review}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("review accepted a different draft", err)
	}
	if _, err = a.Lifecycle(ctx, "api", LifecycleRequest{Action: "pause", Expected: record.Plan.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Apply(ctx, ApplyRequest{Manifest: edited, Review: preview.Review}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("review ignored lifecycle change", err)
	}
	preview, err = a.Apply(ctx, ApplyRequest{Manifest: edited, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Apply(ctx, ApplyRequest{Manifest: edited, Review: preview.Review}); err != nil {
		t.Fatal(err)
	}
}
func TestDestinationOnlyReviewReportsChangeAndPreconditions(t *testing.T) {
	a, _ := setup(t)
	m := `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: console}
spec: {type: console}
`
	p, err := a.Apply(ctx, ApplyRequest{Manifest: m, DryRun: true})
	if err != nil || len(p.Changes) != 0 || len(p.DestinationChanges) != 1 || p.DestinationChanges[0].State != "created" {
		t.Fatal(p, err)
	}
	if _, err = a.Apply(ctx, ApplyRequest{Manifest: m, Review: p.Review}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Apply(ctx, ApplyRequest{Manifest: m, Review: p.Review}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("create precondition ignored", err)
	}
}
