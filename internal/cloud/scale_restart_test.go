package cloud

import (
	"context"
	"reflect"
	"testing"

	state "github.com/ding-labs/ding/internal/serverstate"
)

// Reopen all stores and workers with the existing local encryption key and
// credentials. This is a graceful local restart, not a power-loss simulation.
func scaleRestart(t *testing.T, lab *scaleLab, c scaleConfig) {
	t.Helper()
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
	t.Cleanup(func() { pool.Close() })
	lab.server.DB, lab.server.Vault, lab.server.Pool = db, vault, pool
	for i, tenant := range lab.tenants {
		next, err := scaleTenant(pool, tenant.Workspace, c.Defaults)
		if err != nil {
			t.Fatal(err)
		}
		lab.tenants[i] = next
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
