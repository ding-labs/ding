package state

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVersion100MigrationPreservesLegacyServiceAndQuarantineRevokesKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "control.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schema + "PRAGMA user_version=100;"); err != nil {
		t.Fatal(err)
	}
	workspace := ID()
	service := ID()
	token := "ding_service_" + ID()
	if _, err = db.Exec("INSERT INTO workspaces(id,name,created_at) VALUES(?,?,?)", workspace, "legacy", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO service_grants(id,hash,name,workspace,created_by,scopes,watches,expires_at) VALUES(?,?,?,?,?,?,?,?)", service, TokenHash(token), "legacy", workspace, "owner", `["inspect"]`, `[]`, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	db.Close()
	upgraded, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	grant, err := upgraded.ServiceToken(context.Background(), token)
	if err != nil || grant.ID != service {
		t.Fatal("lost old service", err)
	}
	keys, err := upgraded.ServiceKeys(context.Background(), workspace, service)
	if err != nil || len(keys) != 1 {
		t.Fatal(keys, err)
	}
	if err := upgraded.QuarantineRestore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := upgraded.ServiceToken(context.Background(), token); err == nil {
		t.Fatal("restored key remained usable")
	}
	keys, err = upgraded.ServiceKeys(context.Background(), workspace, service)
	if err != nil || len(keys) != 0 {
		t.Fatal("orphaned restored keys", keys, err)
	}
}
func TestServerMigrationFailureIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "control.db")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schema + "PRAGMA user_version=100; CREATE TABLE webhook_connections(id TEXT);"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if opened, err := Open(context.Background(), dir); err == nil {
		opened.Close()
		t.Fatal("invalid migration succeeded")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 100 {
		t.Fatal("partial version", version, err)
	}
	_, err = db.Exec("SELECT * FROM service_keys")
	if err == nil || !strings.Contains(err.Error(), "no such table") {
		t.Fatal("partial key migration survived", err)
	}
}
