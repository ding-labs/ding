package database

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"testing"
)

func TestPostgresConnectionPolicy(t *testing.T) {
	for _, dsn := range []string{"postgres://u:secret@127.0.0.1/db?sslmode=disable", "postgres://u:secret@db.example/db?sslmode=verify-full"} {
		cfg, err := postgresConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ConnectTimeout.Seconds() != 5 || cfg.RuntimeParams["synchronous_commit"] != "on" {
			t.Fatal("missing connection bounds or durability")
		}
	}
	for _, dsn := range []string{"postgres://u:secret@db.example/db?sslmode=disable", "postgres://u:secret@db.example/db?sslmode=require", "postgres://u:secret@db.example/db?sslmode=prefer", "postgres://u:secret@host:not-a-port/db", "host=127.0.0.1,db.example user=u password=secret dbname=db sslmode=disable"} {
		_, err := postgresConfig(dsn)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe DSN accepted or exposed: %v", err)
		}
	}
}

func TestPostgresErrorsRedacted(t *testing.T) {
	for _, err := range []error{errors.New("password=secret"), &pgconn.PgError{Code: "23505", Message: "secret", Detail: "secret"}} {
		got := SafeError("query", err)
		if strings.Contains(got.Error(), "secret") {
			t.Fatal("credential exposed")
		}
	}
	if !errors.Is(SafeError("query", context.Canceled), context.Canceled) {
		t.Fatal("lost cancellation")
	}
}
