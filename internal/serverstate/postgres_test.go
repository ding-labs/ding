package state

import (
	"context"
	"github.com/ding-labs/ding/internal/pgtest"
	"testing"
)

func postgresTestDB(t *testing.T) *DB {
	t.Helper()
	fixture := pgtest.New(t)
	if err := MigratePostgres(context.Background(), fixture.Migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	db, err := OpenPostgres(context.Background(), fixture.App)
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
