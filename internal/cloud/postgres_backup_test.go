package cloud

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPostgresEncryptedNativeBackup(t *testing.T) {
	if os.Getenv("DING_PG_DUMP") == "" {
		t.Skip("requires matching native PostgreSQL tools")
	}
	f := pgtest.New(t)
	f.SetAppEnvironment(t, "BACKUP_APP")
	f.SetAdminEnvironment(t, "BACKUP_ADMIN")
	c := Config{DataDir: t.TempDir(), Database: database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "BACKUP_APP"}, MaxOpenConns: 1}}
	ctx := context.Background()
	if err := MigrateDatabase(ctx, c, "BACKUP_ADMIN"); err != nil {
		t.Fatal(err)
	}
	f.SetMigrationEnvironment(t, "BACKUP_NON_BYPASS")
	denied := filepath.Join(t.TempDir(), "denied.age")
	rejectedKey, _ := age.GenerateX25519Identity()
	if err := BackupPostgres(ctx, c, "BACKUP_NON_BYPASS", denied, rejectedKey.Recipient().String()); err == nil {
		t.Fatal("non-bypass operator accepted")
	}
	if _, err := os.Stat(denied); !os.IsNotExist(err) {
		t.Fatal("denied backup created output")
	}
	key, _ := age.GenerateX25519Identity()
	output := filepath.Join(t.TempDir(), "backup.age")
	if err := BackupPostgres(ctx, c, "BACKUP_ADMIN", output, key.Recipient().String()); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	dec, err := age.Decrypt(file, key)
	if err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(dec)
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var h postgresBackupHeader
	if json.Unmarshal(line, &h) != nil || h.Format != "ding-postgres/v1" || h.Instance == "" {
		t.Fatal("invalid header")
	}
	magic := make([]byte, 5)
	if _, err = io.ReadFull(r, magic); err != nil || string(magic) != "PGDMP" {
		t.Fatal("native archive missing", err)
	}
	if _, err = io.Copy(io.Discard, r); err != nil {
		t.Fatal("encrypted archive authentication", err)
	}
	if err = BackupPostgres(ctx, c, "BACKUP_ADMIN", output, key.Recipient().String()); err == nil {
		t.Fatal("overwrote existing backup")
	}
	// A tool error may contain connection details; no child output escapes.
	t.Setenv("DING_PG_DUMP", "/usr/bin/false")
	failed := filepath.Join(t.TempDir(), "failed.age")
	if err = BackupPostgres(ctx, c, "BACKUP_ADMIN", failed, key.Recipient().String()); err == nil {
		t.Fatal("failed dump succeeded")
	}
	if _, err = os.Stat(failed); !os.IsNotExist(err) {
		t.Fatal("partial output retained")
	}
}
