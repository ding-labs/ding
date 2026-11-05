package watchrun

import (
	"errors"
	"testing"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
)

func TestPurgeLifecycleRequiresDeletedWatchAndCurrentReview(t *testing.T) {
	a, _ := setup(t)
	r := apply(t, a, "https://example.com")
	if _, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "purge"}); err == nil {
		t.Fatal("purged running watch")
	}
	deleted, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "delete"})
	if err != nil {
		t.Fatal(err)
	}
	viewer := access.WithPrincipal(ctx, access.Principal{User: "viewer", Workspace: "w", Role: access.Viewer, Kind: "browser"})
	if _, err := a.Lifecycle(viewer, "api", LifecycleRequest{Action: "purge"}); !errors.Is(err, access.ErrDenied) {
		t.Fatal(err)
	}
	for _, request := range []LifecycleRequest{
		{Action: "purge", Expected: "stale"},
		{Action: "purge", ExpectedGeneration: &r.Generation},
	} {
		if _, err := a.Lifecycle(ctx, "api", request); !errors.Is(err, store.ErrConflict) {
			t.Fatal(err)
		}
	}
	if _, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "purge", Expected: deleted.Plan.Revision, ExpectedGeneration: &deleted.Generation}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Record(ctx, "api"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	list, err := a.List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: manifest("https://example.com")}); err == nil {
		t.Fatal("reused purged ID")
	}
}
