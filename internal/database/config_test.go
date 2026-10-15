package database

import "testing"

func TestExplicitBackendSelection(t *testing.T) {
	lookup := func(string) (string, bool) { return "secret", true }
	for _, c := range []Config{{}, {Backend: "sqlite"}} {
		if c.Effective().Backend != "sqlite" {
			t.Fatal("SQLite default changed")
		}
		if dsn, err := c.ResolveDSN(lookup); err != nil || dsn != "" {
			t.Fatalf("environment selected backend: %q %v", dsn, err)
		}
	}
	c := Config{Backend: "postgres", DSNRef: &SecretRef{Env: "DING_DATABASE_URL"}}
	if c.Effective().MaxOpenConns != 8 {
		t.Fatal("unbounded default pool")
	}
	if dsn, err := c.ResolveDSN(lookup); err != nil || dsn != "secret" {
		t.Fatal("explicit PostgreSQL selection failed")
	}
	if _, err := c.ResolveDSN(func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("missing secret accepted")
	}
}

func TestRejectInvalidDatabaseOptions(t *testing.T) {
	for _, c := range []Config{
		{Backend: "other"}, {Backend: "sqlite", MaxOpenConns: 8}, {DSNRef: &SecretRef{Env: "DING_DATABASE_URL"}},
		{Backend: "postgres"}, {Backend: "postgres", DSNRef: &SecretRef{Env: "bad-name"}},
		{Backend: "postgres", DSNRef: &SecretRef{Env: "DATABASE_URL"}, MaxOpenConns: -1},
		{Backend: "postgres", DSNRef: &SecretRef{Env: "DATABASE_URL"}, MaxOpenConns: 65},
	} {
		if c.Validate() == nil {
			t.Fatalf("accepted %#v", c)
		}
	}
}
