package watchrun

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
)

func TestDestinationSetupConfirmationIsInteractiveAndCurrent(t *testing.T) {
	testDestinationSetupConfirmation(t, setup)
}
func TestPostgresDestinationSetupConfirmationIsInteractiveAndCurrent(t *testing.T) {
	testDestinationSetupConfirmation(t, postgresSetup)
}
func testDestinationSetupConfirmation(t *testing.T, setup func(*testing.T) (*App, string)) {
	a, dir := setup(t)
	now := a.Now()
	owner := access.Principal{User: "owner", Workspace: "workspace", Role: access.Owner, Kind: "browser"}
	if a.Integration.Workspace != "" {
		owner.Workspace = a.Integration.Workspace
	}
	browser := access.WithPrincipal(context.Background(), owner)
	model := owner
	model.Kind = "integration"
	model.Grant = "grant"
	model.Scopes = []string{"inspect", "manage"}
	modelCtx := access.WithPrincipal(context.Background(), model)
	credential := "https://example.com/private-hook"
	a.Lookup = func(string) (string, bool) { return credential, credential != "" }
	sent := 0
	a.HTTP.Client = &http.Client{Transport: readinessTransport(func(*http.Request) (*http.Response, error) {
		sent++
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	result, err := a.Apply(browser, ApplyRequest{Manifest: "apiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: alerts}\nspec: {type: webhook, urlRef: {env: HOOK}}\n"})
	if err != nil {
		t.Fatal(err)
	}
	req := DestinationTestRequest{AccountID: owner.User, WorkspaceID: owner.Workspace, DestinationRef: "alerts", Revision: result.DestinationChanges[0].Revision, OperationKey: "setup-confirmation-0001"}
	check := func(ctx context.Context, want string) {
		t.Helper()
		s, err := a.DestinationSetupStatus(ctx, "alerts")
		if err != nil || s.State != want {
			t.Fatal(want, s, err)
		}
	}
	check(modelCtx, "untested")
	for range 2 {
		p, err := a.TestDestination(browser, req)
		if err != nil || p.Outcome != "accepted" {
			t.Fatal(p, err)
		}
	}
	check(modelCtx, "accepted")
	if _, err := a.ConfirmDestinationReceipt(modelCtx, req); !errors.Is(err, access.ErrDenied) {
		t.Fatal("model confirmed receipt", err)
	}
	other := owner
	other.User = "other"
	check(access.WithPrincipal(context.Background(), other), "untested")
	if _, err := a.ConfirmDestinationReceipt(access.WithPrincipal(context.Background(), other), req); !errors.Is(err, access.ErrDenied) {
		t.Fatal("other account confirmed", err)
	}
	for range 2 {
		if _, err := a.ConfirmDestinationReceipt(browser, req); err != nil {
			t.Fatal(err)
		}
	}
	check(modelCtx, "confirmed")
	if err := a.Store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := reopenTestStore(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lookup := a.Lookup
	a = New(db)
	a.Now = func() time.Time { return now }
	a.Lookup = lookup
	check(modelCtx, "confirmed")
	credential = "https://example.com/rotated"
	check(modelCtx, "stale")
	if _, err := a.ConfirmDestinationReceipt(browser, req); !errors.Is(err, store.ErrConflict) {
		t.Fatal("rotation accepted", err)
	}
	credential = ""
	check(modelCtx, "unavailable")
	credential = "https://example.com/private-hook"
	a.Now = func() time.Time { return now.Add(store.DestinationConfirmationTTL) }
	check(modelCtx, "expired")
	if sent != 1 {
		t.Fatal("duplicate test", sent)
	}
}

type readinessTransport func(*http.Request) (*http.Response, error)

func (f readinessTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
