package cloud

import (
	"context"
	"reflect"
	"testing"
	"time"

	state "github.com/ding-labs/ding/internal/serverstate"
)

// Reopen all stores and workers with the existing local encryption key and
// credentials. This is a graceful local restart, not a power-loss simulation.
func scaleRestart(t *testing.T, lab *scaleLab, c scaleConfig) {
	t.Helper()
	// Freeze delivery and maintenance before the first snapshot. Their normal
	// progress must not race either side of the durable restart comparison.
	for _, tenant := range lab.tenants {
		tenant.stop()
	}
	for _, tenant := range lab.tenants {
		<-tenant.done
	}
	before := scaleAccounts(t, lab)
	old := lab.server.Pool
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lab.storage.Close(); err != nil {
		t.Fatal(err)
	}
	storage, err := openScaleStorage(context.Background(), lab.root, lab.databaseConfig, c.CredentialReaders)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { storage.Close() })
	db := storage.Control
	lab.storage = storage
	vault, err := state.NewVault(db, lab.key)
	if err != nil {
		t.Fatal(err)
	}
	pool := NewPool(storage.Context(), lab.root, db, vault, old.http)
	storage.ConfigurePool(pool)
	pool.Capacity = old.Capacity
	if lab.receiver != nil {
		// Keep the fixture receiver's expanded delivery capacity across restart.
		// NewPool otherwise reintroduces the production per-host rate limiter.
		pool.http.Transport = scaleDeliveryTransport{receiver: lab.receiver, next: pool.http.Transport}
	}
	t.Cleanup(func() { pool.Close() })
	lab.server.DB, lab.server.Vault, lab.server.Pool = db, vault, pool
	var prepared []*Tenant
	defer func() {
		// Also release cleanup waiters if an assertion exits the test early.
		for _, tenant := range prepared {
			startScaleTenant(tenant)
		}
	}()
	for i, tenant := range lab.tenants {
		next, err := prepareScaleTenant(pool, tenant.Workspace, c.Defaults, c.DeliveryWorkers)
		if err != nil {
			t.Fatal(err)
		}
		lab.tenants[i] = next
		prepared = append(prepared, next)
	}
	after := scaleAccounts(t, lab)
	for i := range before {
		// Startup/clock and maintenance bookmarks legitimately change retained
		// bytes. Compare durable row counts here; report storage separately.
		oldRows, newRows := before[i].Rows, after[i].Rows
		oldRows.Bytes, newRows.Bytes = 0, 0
		if !reflect.DeepEqual(before[i].Usage, after[i].Usage) || !reflect.DeepEqual(oldRows, newRows) {
			t.Fatalf("durable state changed on restart: before=%+v after=%+v", before[i], after[i])
		}
	}
}

func TestScaleRestartPreservesQueuedDeliveries(t *testing.T) {
	testScaleRestartDeliveries(t, true)
}

func TestScaleRestartResumesFixtureReceiverCapacity(t *testing.T) {
	testScaleRestartDeliveries(t, false)
}

func testScaleRestartDeliveries(t *testing.T, recoverBefore bool) {
	t.Helper()
	c := scaleConfig{Name: "restart-deliveries", Rate: 1, Seconds: 1, Outputs: 100, Condition: "alerts", Receiver: "outage"}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	lab.receiver.setRecovery(t, time.Now().Add(time.Hour))
	if status, _ := scaleSend(t, lab, lab.producers[0], "queued-before-restart", lab.producers[0].Body); status != 202 {
		t.Fatal("fixture acceptance", status)
	}
	if scaleAccounts(t, lab)[0].Rows.Pending != 100 {
		t.Fatal("fixture must have queued deliveries")
	}
	// Let delivery resume during the restart. Only the quiescent snapshots may
	// be compared; resumed workers are expected to change pending row counts.
	if recoverBefore {
		lab.receiver.setRecovery(t, time.Now())
	}
	scaleRestart(t, lab, c)
	if !recoverBefore {
		lab.receiver.setRecovery(t, time.Now())
	}
	deadline := time.Now().Add(20 * time.Second)
	for scaleAccounts(t, lab)[0].Rows.Pending != 0 {
		if time.Now().After(deadline) {
			t.Fatal("queued deliveries did not recover")
		}
		time.Sleep(50 * time.Millisecond)
	}
	receiver, problems := scaleReceiverEvidence(t, lab, t.TempDir())
	if len(problems) != 0 || len(receiver.IDs) != 100 {
		t.Fatal("restart lost receiver evidence", problems, len(receiver.IDs))
	}
}
