package database

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func postgresFixture(t *testing.T, role string) Config {
	t.Helper()
	path := os.Getenv("DING_TEST_POSTGRES_FILE")
	if path == "" {
		t.Skip("set DING_TEST_POSTGRES_FILE to an isolated private lab connection file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read isolated PostgreSQL fixture")
	}
	var values map[string]string
	if json.Unmarshal(b, &values) != nil || values[role] == "" {
		t.Fatal("invalid isolated PostgreSQL fixture")
	}
	t.Setenv("DING_TEST_POSTGRES_DSN", values[role])
	return Config{Backend: "postgres", DSNRef: &SecretRef{Env: "DING_TEST_POSTGRES_DSN"}, MaxOpenConns: 1}
}

func TestPostgresMigrationTransaction(t *testing.T) {
	cfg := postgresFixture(t, "migration")
	ctx := context.Background()
	db, err := OpenPostgres(ctx, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// A failed component migration must not publish its table or version journal.
	err = Migrate(ctx, db, "runtime", []string{"CREATE SCHEMA ding_runtime; CREATE TABLE ding_runtime.failed_probe(id integer); SELECT nonexistent_migration_function();"}, "ding_app")
	if err == nil {
		t.Fatal("bad migration succeeded")
	}
	var absent bool
	if err = db.QueryRow("SELECT to_regclass('ding_runtime.failed_probe') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatal("failed migration leaked state")
	}
}
