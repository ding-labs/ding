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
	if err := lab.server.DB.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := state.Open(context.Background(), lab.root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	vault, err := state.NewVault(db, lab.key)
	if err != nil {
		t.Fatal(err)
	}
	pool := NewPool(context.Background(), lab.root, db, vault, old.http)
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
		if !reflect.DeepEqual(before[i].Usage, after[i].Usage) || !reflect.DeepEqual(before[i].Rows, after[i].Rows) {
			t.Fatal("durable state changed on restart")
		}
	}
}
