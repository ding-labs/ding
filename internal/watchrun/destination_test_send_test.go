package watchrun

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/notify"
	"github.com/ding-labs/ding/internal/store"
)

func TestExistingDestinationProbeUsesExactHeadersAndDoesNotResend(t *testing.T) {
	app, dir := setup(t)
	var sent atomic.Int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Secret") != "private-value" || !strings.Contains(string(body), "not an incident") {
			t.Error("missing configured header or labeled test")
		}
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	manifest := "apiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: existing}\nspec: {type: webhook, urlRef: {env: ENDPOINT}, headers: {X-Secret: {env: HEADER}}}\n"
	result, err := app.Apply(ctx, ApplyRequest{Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	request := DestinationTestRequest{OperationKey: "existing-test-key-0001", DestinationRef: "existing", Revision: result.DestinationChanges[0].Revision}
	header := "private-value"
	lookup := func(name string) (string, bool) {
		if name == "ENDPOINT" {
			return receiver.URL, true
		}
		return header, name == "HEADER"
	}
	app.Lookup = lookup
	probe, err := app.TestDestination(ctx, request)
	if err != nil || probe.Outcome != "accepted" {
		t.Fatal(probe, err)
	}
	if err := app.Store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app = New(db)
	app.Lookup = lookup
	probe, err = app.TestDestination(ctx, request)
	if err != nil || probe.Outcome != "accepted" || sent.Load() != 1 {
		t.Fatal("replayed test resent", probe, err, sent.Load())
	}
	header = "rotated-value"
	if _, err := app.TestDestination(ctx, request); !errors.Is(err, store.ErrOperationConflict) {
		t.Fatal("changed credential reused proof", err)
	}
	request.Revision = "stale"
	if _, err := app.TestDestination(ctx, request); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale destination tested", err)
	}
	denied := access.WithPrincipal(context.Background(), access.Principal{User: "viewer", Workspace: "w", Role: access.Viewer})
	if _, err := app.TestDestination(denied, request); !errors.Is(err, access.ErrDenied) {
		t.Fatal("viewer sent test", err)
	}
	if sent.Load() != 1 {
		t.Fatal("rejected request sent a notification")
	}
}

func TestDestinationProbePreservesUncertainDelivery(t *testing.T) {
	app, _ := setup(t)
	result, err := app.Apply(ctx, ApplyRequest{Manifest: "apiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: desktop}\nspec: {type: desktop}\n"})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	app.Notify = func(context.Context, notify.Message) error { count++; return errors.New("response lost") }
	request := DestinationTestRequest{OperationKey: "uncertain-test-0001", DestinationRef: "desktop", Revision: result.DestinationChanges[0].Revision}
	for range 2 {
		probe, err := app.TestDestination(ctx, request)
		if err != nil || probe.Outcome != "pending" {
			t.Fatal(probe, err)
		}
	}
	if count != 1 {
		t.Fatal("uncertain test resent")
	}
}
