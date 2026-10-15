package state

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/ding-labs/ding/internal/database"
)

func postgresTestDB(t *testing.T) *DB {
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
	if json.Unmarshal(data, &values) != nil {
		t.Fatal("invalid PostgreSQL fixture")
	}
	cfg := database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "FIXTURE"}, MaxOpenConns: 1}
	ctx := context.Background()
	migration, err := database.OpenPostgres(ctx, cfg, func(string) (string, bool) { return values["migration"], values["migration"] != "" })
	if err != nil {
		t.Fatal(err)
	}
	err = MigratePostgres(ctx, migration, "ding_app")
	migration.Close()
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.OpenPostgres(ctx, cfg, func(string) (string, bool) { return values["app"], values["app"] != "" })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	db, err := OpenPostgres(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPostgresControlSchemaAndRoleBoundary(t *testing.T) {
	d := postgresTestDB(t)
	var n int
	if err := d.sql.QueryRow("SELECT count(*) FROM ding_control.writer WHERE singleton=1").Scan(&n); err != nil || n != 1 {
		t.Fatal("control writer fence missing", err)
	}
	if _, err := d.sql.Exec("CREATE TABLE ding_control.unapproved_table(id integer)"); err == nil {
		t.Fatal("application role owns schema")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := d.sql.Ping(); err != nil {
		t.Fatal("control facade closed shared pool")
	}
}
