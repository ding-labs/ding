package cloud

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/database"
)

func TestDatabaseConfigPreservesSQLiteDefaultAndFailsWithoutFallback(t *testing.T) {
	dir := t.TempDir()
	storage, err := OpenServerStorage(context.Background(), dir, database.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "control.db")); err != nil {
		t.Fatal("omitted backend did not select SQLite")
	}
	if err = storage.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DING_INVALID_DATABASE_URL", "postgres://synthetic-secret@host:not-a-port/db")
	failed := t.TempDir()
	if _, err = OpenServerStorage(context.Background(), failed, database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "DING_INVALID_DATABASE_URL"}}); err == nil {
		t.Fatal("invalid DSN accepted")
	}
	if _, err = os.Stat(filepath.Join(failed, "control.db")); !os.IsNotExist(err) {
		t.Fatal("failed PostgreSQL configuration fell back to SQLite")
	}
	if _, err = OpenServerStorage(context.Background(), dir, database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "DING_INVALID_DATABASE_URL"}}); err == nil {
		t.Fatal("existing SQLite state implicitly switched backend")
	}
}
