package watchrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/store"
)

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
	return a, t.TempDir()
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
