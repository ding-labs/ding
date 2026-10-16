package railway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresStartupSecretBoundary(t *testing.T) {
	env := environment()
	env["DING_DATABASE_JSON"] = `{"backend":"postgres","dsnRef":{"env":"DING_DATABASE_URL"},"maxOpenConns":16}`
	env["DING_DATABASE_URL"] = "postgres://app:private-password@postgres.railway.internal/ding?sslmode=verify-full"
	env["DING_DATABASE_CA_PEM"] = "public certificate"
	s, _, _, err := FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s)
	if err != nil || strings.Contains(string(b), "private-password") || s.DatabaseDSN != env["DING_DATABASE_URL"] {
		t.Fatal("secret serialization boundary failed")
	}
	for _, name := range []string{"DING_DATABASE_URL", "DING_DATABASE_JSON", "DING_DATABASE_CA_PEM"} {
		if !SecretEnvironment(name) {
			t.Fatal("database input survives clean exec", name)
		}
	}
	t.Setenv("DING_DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "dsn")
	if err := os.WriteFile(path, []byte(s.DatabaseDSN), 0600); err != nil {
		t.Fatal(err)
	}
	if err := restoreDatabaseEnvironment(s.Config.Database, path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DING_DATABASE_URL") != s.DatabaseDSN {
		t.Fatal("DSN was not restored")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := restoreDatabaseEnvironment(s.Config.Database, path); err == nil {
		t.Fatal("accepted public DSN file")
	}
}

func TestDatabaseStartupFailsClosed(t *testing.T) {
	for _, raw := range []string{
		`{"backend":"postgres","dsnRef":{"env":"OTHER_SECRET"}}`,
		`{"backend":"postgres","dsnRef":{"env":"DING_DATABASE_URL"}}`,
		`{"backend":"postgres","password":"secret"}`,
		`{"backend":"sqlite"} {}`,
	} {
		env := environment()
		env["DING_DATABASE_JSON"] = raw
		if _, _, _, err := FromEnv(func(k string) string { return env[k] }); err == nil {
			t.Fatal("accepted invalid database settings", raw)
		}
	}
	env := environment()
	env["DING_DATABASE_URL"] = "unexpected"
	if _, _, _, err := FromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("implicitly enabled PostgreSQL")
	}
	s, _, _, err := FromEnv(func(k string) string { return environment()[k] })
	if err != nil || s.Config.Database.Effective().Backend != "sqlite" {
		t.Fatal("SQLite default changed", err)
	}
}
