package watchrun

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/store"
)

func TestServicePreviewPreservesReviewAndReceipts(t *testing.T) { testServicePreview(t, setup) }
func TestPostgresServicePreviewPreservesReviewAndReceipts(t *testing.T) {
	testServicePreview(t, postgresSetup)
}

func testServicePreview(t *testing.T, setup func(*testing.T) (*App, string)) {
	a, dir := setup(t)
	const destination = "apiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: alerts}\nspec: {type: webhook, urlRef: {env: WEBHOOK_URL}}\n"
	d, err := a.Apply(ctx, ApplyRequest{Manifest: destination})
	if err != nil {
		t.Fatal(err)
	}
	g := integrationGrant(t, a, "inspect", "preview", "manage")
	m, err := onboarding.ServiceManifest(onboarding.ServiceRequest{ID: "service", URL: "https://example.com/health", DestinationRef: "alerts", ExpectedStatus: new(204)}, "cloud")
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.PreviewNewServiceIntegration(ctx, g.Grant.ID, m)
	if err != nil || p.Changes.Review.Destinations["alerts"] != d.DestinationChanges[0].Revision {
		t.Fatal("destination not bound", err, p)
	}
	if _, err := a.Record(ctx, "service"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("preview created watch", err)
	}
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: strings.Replace(destination, "type: webhook", "type: slack", 1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, "service_stale_key_001"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale destination accepted", err)
	}
	p, err = a.PreviewNewServiceIntegration(ctx, g.Grant.ID, m)
	if err != nil {
		t.Fatal(err)
	}
	other := integrationGrant(t, a, "inspect", "preview", "manage")
	if _, err := a.ApplyIntegration(ctx, other.Grant.ID, p.Handle, "service_other_key_001"); !errors.Is(err, store.ErrPreviewExpired) {
		t.Fatal("cross-grant preview accepted", err)
	}
	readOnly := integrationGrant(t, a, "inspect")
	if _, err := a.PreviewNewServiceIntegration(ctx, readOnly.Grant.ID, m); !errors.Is(err, store.ErrIntegrationDenied) {
		t.Fatal("read-only grant previewed", err)
	}
	// The typed starter produces the same durable handle used by ordinary apply.
	if err := a.Store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := reopenTestStore(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a = New(db)
	a.Now = func() time.Time { return p.ExpiresAt.Add(-time.Minute) }
	want, err := a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, "service_apply_key_001")
	if err != nil {
		t.Fatal(err)
	}
	again, err := a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, "service_apply_key_001")
	if err != nil || !reflect.DeepEqual(want, again) {
		t.Fatal("duplicate apply", err)
	}
	if _, err := a.PreviewNewServiceIntegration(ctx, g.Grant.ID, m); err == nil {
		t.Fatal("starter accepted existing watch ID")
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error {
		page, err := tx.EventPage("service", "", 100)
		if len(page.Events) != 1 {
			t.Errorf("unexpected watch effects: %d", len(page.Events))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// A watch created after preview must not be overwritten by starter activation.
	m2 := strings.Replace(m, "id: service", "id: racing", 1)
	p2, err := a.PreviewNewServiceIntegration(ctx, g.Grant.ID, m2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m2}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ApplyIntegration(ctx, g.Grant.ID, p2.Handle, "service_racing_key_001"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("starter overwrote intervening watch", err)
	}
}
