package watchrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
)

func integrationGrant(t *testing.T, a *App, scopes ...string) GrantPairing {
	t.Helper()
	g, err := a.CreateIntegrationGrant(ctx, GrantRequest{Name: "test", Scopes: scopes, SecretRefs: []string{"WEBHOOK_URL"}, Days: 90})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestIntegrationConcurrentApplyReceiptsSurviveRestartAndExpiry(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a := New(s)
	g := integrationGrant(t, a, "inspect", "preview", "manage")
	p, err := a.PreviewIntegration(ctx, g.Grant.ID, manifest("https://example.com"))
	if err != nil {
		t.Fatal(err)
	}
	key := "apply_operation_000001"
	var wg sync.WaitGroup
	results := make([]ApplyResult, 8)
	failures := make([]error, 8)
	for i := range results {
		wg.Go(func() { results[i], failures[i] = a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, key) })
	}
	wg.Wait()
	for i, err := range failures {
		if err != nil || !reflect.DeepEqual(results[0], results[i]) {
			t.Fatalf("retry %d: %v", i, err)
		}
	}
	var events int
	if err = a.Store.View(ctx, func(tx *store.Tx) error { page, e := tx.EventPage("api", "", 100); events = len(page.Events); return e }); err != nil || events != 1 {
		t.Fatalf("duplicate effect: %d, %v", events, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a = New(s)
	a.Now = func() time.Time { return p.ExpiresAt.Add(time.Minute) }
	again, err := a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, key)
	if err != nil || !reflect.DeepEqual(results[0], again) {
		t.Fatal("receipt lost after restart/preview expiry", err)
	}
	if _, err = a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, "new_operation_000002"); !errors.Is(err, store.ErrPreviewExpired) {
		t.Fatal(err)
	}
	if _, err = a.ApplyIntegration(ctx, g.Grant.ID, strings.Repeat("b", 64), key); !errors.Is(err, store.ErrOperationConflict) {
		t.Fatal("key reused for different request", err)
	}
	if err = s.Update(ctx, func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(g.Grant.ID) }); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, key); !errors.Is(err, store.ErrIntegrationDenied) {
		t.Fatal("revoked grant replayed receipt", err)
	}
}

func TestIntegrationPreviewIsolationStalenessAndScopes(t *testing.T) {
	a, _ := setup(t)
	g := integrationGrant(t, a, "inspect", "preview", "manage")
	other := integrationGrant(t, a, "inspect", "preview", "manage")
	p, err := a.PreviewIntegration(ctx, g.Grant.ID, manifest("https://example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyIntegration(ctx, other.Grant.ID, p.Handle, "operation_other_00001"); !errors.Is(err, store.ErrPreviewExpired) {
		t.Fatal("cross-principal preview", err)
	}
	readOnly := integrationGrant(t, a, "inspect")
	if _, err = a.PreviewIntegration(ctx, readOnly.Grant.ID, manifest("https://example.com")); !errors.Is(err, store.ErrIntegrationDenied) {
		t.Fatal("read-only preview", err)
	}
	if _, err = a.Apply(ctx, ApplyRequest{Manifest: manifest("https://other.example.com")}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, "operation_stale_00001"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale preview", err)
	}
	if _, err = a.GetIntegrationOperation(ctx, g.Grant.ID, "operation_stale_00001"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("failed operation has success receipt", err)
	}
}

func TestIntegrationLifecycleGenerationAndReceiptBinding(t *testing.T) {
	a, _ := setup(t)
	r := apply(t, a, "https://example.com")
	g := integrationGrant(t, a, "inspect", "manage")
	req := LifecycleRequest{Action: "pause", Expected: r.Plan.Revision, ExpectedGeneration: &r.Generation}
	paused, err := a.LifecycleIntegration(ctx, g.Grant.ID, "api", "operation_pause_00001", req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := a.LifecycleIntegration(ctx, g.Grant.ID, "api", "operation_pause_00001", req)
	if err != nil || again.Generation != paused.Generation {
		t.Fatal("duplicate lifecycle effect", err)
	}
	req.Action = "resume"
	if _, err = a.LifecycleIntegration(ctx, g.Grant.ID, "api", "operation_pause_00001", req); !errors.Is(err, store.ErrOperationConflict) {
		t.Fatal(err)
	}
	if _, err = a.LifecycleIntegration(ctx, g.Grant.ID, "api", "operation_resume_0001", req); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale generation", err)
	}
	req.ExpectedGeneration = &paused.Generation
	if _, err = a.LifecycleIntegration(ctx, g.Grant.ID, "api", "operation_resume_0001", req); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationCommandRevisionsRequireExplicitLocalGrant(t *testing.T) {
	a, _ := setup(t)
	manifest := fmt.Sprintf(`apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: command}
spec:
  source: {type: command, argv: [echo, '{"n": 1}'], directory: %q}
  condition: {field: n, operator: eq, value: 1}
`, filepath.ToSlash(t.TempDir()))
	g := integrationGrant(t, a, "inspect", "preview", "manage")
	if _, err := a.PreviewIntegration(ctx, g.Grant.ID, manifest); !errors.Is(err, store.ErrIntegrationDenied) {
		t.Fatal("command accepted", err)
	}
	b, err := plan.Parse([]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := a.CreateIntegrationGrant(ctx, GrantRequest{Name: "allowed", Scopes: g.Grant.Scopes, Days: 1, CommandRevisions: []string{b.Watches[0].Revision}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.PreviewIntegration(ctx, allowed.Grant.ID, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyIntegration(ctx, allowed.Grant.ID, p.Handle, "command_apply_000001"); err != nil {
		t.Fatal(err)
	}
	if _, err = a.PreviewIntegration(ctx, allowed.Grant.ID, strings.Replace(manifest, "echo", "env", 1)); !errors.Is(err, store.ErrIntegrationDenied) {
		t.Fatal("changed command accepted", err)
	}
}

func TestIntegrationReceiptFailureRollsBackEffect(t *testing.T) {
	a, _ := setup(t)
	g := integrationGrant(t, a, "inspect", "preview", "manage")
	p, err := a.PreviewIntegration(ctx, g.Grant.ID, manifest("https://example.com"))
	if err != nil {
		t.Fatal(err)
	}
	// Force the real bounded-receipt admission failure, rather than mocking a
	// transaction callback. The attempted watch creation must roll back with it.
	err = a.Store.Update(ctx, func(tx *store.Tx) error {
		for i := 0; i < 10000; i++ {
			key := stringKey(i)
			if err := tx.SaveIntegrationOperation(g.Grant.ID, store.IntegrationOperation{Key: key, Action: "seed", Digest: "d", Result: json.RawMessage(`{}`), CreatedAt: a.Now()}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, "full_receipts_000001"); !errors.Is(err, store.ErrIntegrationLimit) {
		t.Fatal(err)
	}
	if _, err = a.Record(ctx, "api"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("effect committed without receipt", err)
	}
}

func stringKey(i int) string { b, _ := json.Marshal(i); return "seed_receipt_" + string(b) }

func TestIntegrationRetryReceiptDoesNotRequeueAgain(t *testing.T) {
	a, _ := setup(t)
	r := apply(t, a, "https://example.com")
	for n := 1; n <= 3; n++ {
		input(t, a, r, n, 500)
	}
	g := integrationGrant(t, a, "inspect", "retry")
	var id int64
	err := a.Store.Update(ctx, func(tx *store.Tx) error {
		jobs, err := tx.Outbox("api")
		if err != nil {
			return err
		}
		id = jobs[0].ID
		return tx.CancelDeliveries("api", a.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = a.RetryIntegration(ctx, g.Grant.ID, id, "retry_operation_00001"); err != nil {
		t.Fatal(err)
	}
	if err = a.Store.Update(ctx, func(tx *store.Tx) error { return tx.CancelDeliveries("api", a.Now()) }); err != nil {
		t.Fatal(err)
	}
	if err = a.RetryIntegration(ctx, g.Grant.ID, id, "retry_operation_00001"); err != nil {
		t.Fatal(err)
	}
	if err = a.Store.View(ctx, func(tx *store.Tx) error {
		d, err := tx.Intent(id)
		if err != nil {
			return err
		}
		if d.Status != "canceled" {
			t.Fatal("receipt retry repeated external work")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationNewSecretReferencesNeedLocalPermission(t *testing.T) {
	a, _ := setup(t)
	g := integrationGrant(t, a, "inspect", "preview", "manage")
	m := `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: sensitive}
spec:
  source: {type: http, url: https://example.com, headers: {Authorization: {env: PRIVATE_TOKEN}}}
  condition: {field: http.status, operator: eq, value: 200}
`
	if _, err := a.PreviewIntegration(ctx, g.Grant.ID, m); !errors.Is(err, store.ErrIntegrationDenied) {
		t.Fatal("ungranted daemon environment reference", err)
	}
}
