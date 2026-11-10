package watchrun

import (
	"context"
	"slices"
	"testing"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
)

func TestLifecycleGuidance(t *testing.T) {
	app, _ := setup(t)
	apply(t, app, "https://example.com")
	viewer := access.WithPrincipal(ctx, access.Principal{User: "reader", Workspace: "test", Role: access.Viewer, Kind: "browser"})
	manager := integrationGrant(t, app, "inspect", "manage").Grant
	readOnly := integrationGrant(t, app, "inspect").Grant
	check := func(c context.Context, grant *store.IntegrationGrant, reason string, actions ...string) {
		t.Helper()
		detail, err := app.WatchDetail(c, "api", "", 0, grant)
		if err != nil || detail.Lifecycle == nil || detail.Lifecycle.Actions == nil ||
			!slices.Equal(detail.Lifecycle.Actions, actions) || detail.Lifecycle.Reason != reason {
			t.Fatalf("guidance %+v, want %v (%s), err %v", detail.Lifecycle, actions, reason, err)
		}
	}
	check(ctx, nil, "", "pause", "delete")
	check(ctx, &manager, "", "pause", "delete")
	check(viewer, &manager, "manage_unavailable")
	check(ctx, &readOnly, "manage_unavailable")
	if _, err := app.Lifecycle(ctx, "api", LifecycleRequest{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	check(ctx, nil, "", "resume", "delete")
	if _, err := app.Lifecycle(ctx, "api", LifecycleRequest{Action: "delete"}); err != nil {
		t.Fatal(err)
	}
	check(ctx, nil, "", "purge")
	check(ctx, &manager, "")
}

func TestHeldWatchGuidance(t *testing.T) {
	_, target, prepared, _ := transferFixture(t)
	detail, err := target.WatchDetail(ctx, "api", "", 0, nil)
	if err != nil || detail.Lifecycle == nil || detail.Lifecycle.Reason != "transfer_held" ||
		detail.Lifecycle.TransferID != prepared.ID || len(detail.Lifecycle.Actions) != 0 {
		t.Fatalf("prepared target: %+v, %v", detail.Lifecycle, err)
	}
}
