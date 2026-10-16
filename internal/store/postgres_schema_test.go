package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/ding-labs/ding/internal/database"
)

func postgresTestPool(t *testing.T, role string, modes ...string) *sql.DB {
	t.Helper()
	file := os.Getenv("DING_TEST_POSTGRES_FILE")
	if file == "" {
		t.Skip("requires isolated PostgreSQL lab")
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("cannot read private PostgreSQL fixture")
	}
	var values map[string]string
	if json.Unmarshal(b, &values) != nil || values[role] == "" {
		t.Fatal("invalid PostgreSQL fixture")
	}
	mode := "exec"
	if len(modes) > 0 {
		mode = modes[0]
	}
	db, err := database.OpenPostgres(context.Background(), database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "FIXTURE"}, MaxOpenConns: 1, QueryMode: mode}, func(string) (string, bool) { return values[role], true })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestPostgresRecordSchemaIsolation(t *testing.T) {
	ctx := context.Background()
	migration := postgresTestPool(t, "migration")
	if err := MigratePostgres(ctx, migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	app := postgresTestPool(t, "app")
	for _, id := range []string{"schema-a", "schema-b"} {
		tx, err := app.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec("SELECT set_config('ding.workspace',$1,true)", id); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec("INSERT INTO ding_runtime.workspaces(workspace_id) VALUES($1) ON CONFLICT DO NOTHING", id); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec("INSERT INTO ding_runtime.metadata(key,value) VALUES('same-id',$1) ON CONFLICT(workspace_id,key) DO UPDATE SET value=excluded.value", id); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := app.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRow("SELECT count(*) FROM ding_runtime.metadata").Scan(&n); err != nil || n != 0 {
		t.Fatalf("missing scope leaked %d rows: %v", n, err)
	}
	if _, err = tx.Exec("SELECT set_config('ding.workspace','schema-a',true)"); err != nil {
		t.Fatal(err)
	}
	var value string
	if err = tx.QueryRow("SELECT value FROM ding_runtime.metadata WHERE key='same-id'").Scan(&value); err != nil || value != "schema-a" {
		t.Fatal("cross-workspace identity collision")
	}
	if _, err = tx.Exec("INSERT INTO ding_runtime.metadata(workspace_id,key,value) VALUES('schema-b','cross','bad')"); err == nil {
		t.Fatal("cross-workspace write accepted")
	}
}
