package watchrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/store"
)

func TestPostgresIntegrationApplyReceiptSurvivesRestartAndExpiry(t *testing.T) {
	a, dir := postgresSetup(t)
	g := integrationGrant(t, a, "inspect", "preview", "manage")
	p, err := a.PreviewIntegration(ctx, g.Grant.ID, manifest("https://example.com"))
	if err != nil {
		t.Fatal(err)
	}
	const key = "postgres_apply_operation_000001"
	want, err := a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, key)
	if err != nil {
		t.Fatal("MCP apply failed to save its receipt", err)
	}
	if len(want.Changes) != 1 || want.Changes[0].State != "created" {
		t.Fatal("watch not created", want)
	}
	if err := a.Store.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := reopenTestStore(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a = New(s)
	a.Now = func() time.Time { return p.ExpiresAt.Add(time.Minute) }
	again, err := a.ApplyIntegration(ctx, g.Grant.ID, p.Handle, key)
	if err != nil || !reflect.DeepEqual(want, again) {
		t.Fatal("apply receipt changed after restart and preview expiry", err)
	}
	op, err := a.GetIntegrationOperation(ctx, g.Grant.ID, key)
	if err != nil {
		t.Fatal(err)
	}
	var receipt ApplyResult
	if err := json.Unmarshal(op.Result, &receipt); err != nil || !reflect.DeepEqual(want, receipt) {
		t.Fatal("stored apply result lost JSON escapes", err)
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error {
		page, err := tx.EventPage("api", "", 100)
		if len(page.Events) != 1 {
			t.Errorf("retry duplicated the apply event: %d events", len(page.Events))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func postgresSetup(t *testing.T) (*App, string) {
	t.Helper()
	file := os.Getenv("DING_TEST_POSTGRES_FILE")
	if file == "" {
		t.Skip("requires isolated PostgreSQL lab")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("cannot read private PostgreSQL fixture")
	}
	var values map[string]string
	if json.Unmarshal(data, &values) != nil || values["app"] == "" || values["migration"] == "" {
		t.Fatal("invalid PostgreSQL fixture")
	}
	cfg := database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "FIXTURE"}, MaxOpenConns: 1}
	migration, err := database.OpenPostgres(ctx, cfg, func(string) (string, bool) { return values["migration"], true })
	if err != nil {
		t.Fatal(err)
	}
	err = store.MigratePostgres(ctx, migration, "ding_app")
	migration.Close()
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.OpenPostgres(ctx, cfg, func(string) (string, bool) { return values["app"], true })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprint(time.Now().UnixNano()))))
	s, err := store.OpenPostgres(ctx, pool, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a := New(s)
	a.Now = func() time.Time { return start }
	dir := t.TempDir()
	postgresReopen.Store(dir, func(ctx context.Context) (*store.Store, error) { return store.OpenPostgres(ctx, pool, id) })
	t.Cleanup(func() { postgresReopen.Delete(dir) })
	return a, dir
}

func TestPostgresApplicationPushReplayAndQuota(t *testing.T) {
	a, _ := postgresSetup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: receiptFixture}); err != nil {
		t.Fatal(err)
	}
	r, err := a.Record(ctx, "push")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"value":9}`)
	batch, err := PreparePush(ctx, r, raw, 32, 32000)
	if err != nil {
		t.Fatal(err)
	}
	first := NewPushInput("producer", "key", raw, 1, time.Hour)
	receipt, err := a.CommitPush(ctx, r, batch, first)
	if err != nil {
		t.Fatal(err)
	}
	again, err := a.CommitPush(ctx, r, batch, first)
	if err != nil || !again.Duplicate || again.AcceptanceID != receipt.AcceptanceID {
		t.Fatal("duplicate mutated state", err)
	}
	if _, err = a.CommitPush(ctx, r, batch, NewPushInput("producer", "other", raw, 1, time.Hour)); err != ErrQuota {
		t.Fatal("receipt quota bypassed", err)
	}
	if err = a.Maintain(ctx, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = a.Store.View(ctx, func(tx *store.Tx) error {
		usage, err := tx.PushUsage()
		if usage.Requests != 1 || usage.Observations != 1 {
			t.Error("usage mismatch", usage)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresConcurrentPushReceipts(t *testing.T) {
	a, _ := postgresSetup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: receiptFixture}); err != nil {
		t.Fatal(err)
	}
	// One app API is shared by concurrent callers here; the store-level tests also
	// exercise distinct facades against the same workspace lock.
	var wg sync.WaitGroup
	failures := make(chan error, 50)
	identities := make(chan store.PushReceipt, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			record, err := a.Record(context.Background(), "push")
			if err != nil {
				failures <- err
				return
			}
			raw := []byte(`{"value":9}`)
			batch, err := PreparePush(ctx, record, raw, 32, 32000)
			if err != nil {
				failures <- err
				return
			}
			receipt, err := a.CommitPush(ctx, record, batch, NewPushInput("producer", fmt.Sprint(i%10), raw, 100, time.Hour))
			if err != nil {
				failures <- err
				return
			}
			identities <- receipt
		}(i)
	}
	wg.Wait()
	close(failures)
	close(identities)
	for err := range failures {
		t.Fatal(err)
	}
	accepted := map[string]int64{}
	for receipt := range identities {
		if prior, ok := accepted[receipt.AcceptanceID]; ok && prior != receipt.First {
			t.Fatal("replay changed range")
		}
		accepted[receipt.AcceptanceID] = receipt.First
	}
	if len(accepted) != 10 {
		t.Fatal("duplicate requests committed independently", len(accepted))
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error {
		usage, err := tx.PushUsage()
		if usage.Requests != 10 || usage.Observations != 10 {
			t.Error("concurrent usage mismatch", usage)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

var postgresReopen sync.Map

// Shared restart tests reopen the same backend rather than replacing a
// PostgreSQL workspace with a new, empty SQLite directory.
func reopenTestStore(ctx context.Context, dir string) (*store.Store, error) {
	if factory, ok := postgresReopen.Load(dir); ok {
		return factory.(func(context.Context) (*store.Store, error))(ctx)
	}
	return store.Open(ctx, dir)
}
