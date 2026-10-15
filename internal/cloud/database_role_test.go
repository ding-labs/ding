package cloud

import (
	"context"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
)

func TestPostgresApplicationCannotOwnDatabase(t *testing.T) {
	f := pgtest.New(t)
	ctx := context.Background()
	if err := store.MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	if err := state.MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	f.SetAdminEnvironment(t, "ROLE_TEST_ADMIN")
	admin, err := database.OpenPostgres(ctx, database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "ROLE_TEST_ADMIN"}, MaxOpenConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	// Database ownership is unsafe even when the role owns neither Ding schema.
	if _, err = admin.ExecContext(ctx, "ALTER DATABASE "+f.Name+" OWNER TO ding_app"); err != nil {
		t.Fatal(database.SafeError("change local fixture owner", err))
	}
	if err = store.ValidatePostgres(ctx, f.App); err == nil {
		t.Fatal("runtime accepted database owner")
	}
	if db, err := state.OpenPostgres(ctx, f.App); err == nil {
		db.Close()
		t.Fatal("control accepted database owner")
	}
	if err = store.MigratePostgres(ctx, f.Migration, "ding_app"); err == nil {
		t.Fatal("migration accepted database-owning app role")
	}
}
