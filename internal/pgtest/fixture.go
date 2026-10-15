// Package pgtest owns isolated, opt-in PostgreSQL databases for contract tests.
package pgtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/database"
)

type Fixture struct {
	NewAppPool     func(int) *sql.DB
	App, Migration *sql.DB
	Name           string
}

func New(t *testing.T) *Fixture {
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
	if json.Unmarshal(data, &values) != nil || values["admin"] == "" || values["app"] == "" || values["migration"] == "" {
		t.Fatal("invalid PostgreSQL fixture")
	}
	var id [10]byte
	if _, err = rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	name := "dingtest_" + hex.EncodeToString(id[:])
	open := func(role, dbname string) *sql.DB {
		t.Helper()
		u, e := url.Parse(values[role])
		if e != nil || u.Scheme != "postgres" {
			t.Fatal("fixture requires PostgreSQL URL references")
		}
		u.Path = "/" + dbname
		pool, e := database.OpenPostgres(context.Background(), database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "FIXTURE"}, MaxOpenConns: 1}, func(string) (string, bool) { return u.String(), true })
		if e != nil {
			t.Fatal(e)
		}
		return pool
	}
	admin := open("admin", "ding")
	if _, err = admin.Exec("CREATE DATABASE " + name + " OWNER ding_migrator TEMPLATE template0"); err != nil {
		admin.Close()
		t.Fatal(database.SafeError("create isolated fixture", err))
	}
	fixture := &Fixture{Name: name}
	fixture.NewAppPool = func(max int) *sql.DB {
		pool := open("app", name)
		pool.SetMaxOpenConns(max)
		t.Cleanup(func() { pool.Close() })
		return pool
	}
	t.Cleanup(func() {
		if fixture.App != nil {
			fixture.App.Close()
		}
		if fixture.Migration != nil {
			fixture.Migration.Close()
		}
		defer admin.Close()
		if t.Failed() {
			dir := filepath.Join(filepath.Dir(file), "failed-fixtures")
			_ = os.MkdirAll(dir, 0700)
			b, _ := json.Marshal(map[string]string{"database": name, "test": t.Name()})
			_ = os.WriteFile(filepath.Join(dir, name+".json"), b, 0600)
			t.Logf("Failed local PostgreSQL fixture preserved: %s", name)
			return
		}
		if _, err := admin.Exec("DROP DATABASE " + name + " WITH (FORCE)"); err != nil {
			t.Error(database.SafeError("remove owned fixture", err))
		}
	})
	fixture.Migration = open("migration", name)
	fixture.App = open("app", name)
	return fixture
}
