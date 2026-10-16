package watchcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPostgresSharedServerMigrateBootstrapAndRecovery(t *testing.T) {
	fixture := pgtest.New(t)
	fixture.SetAppEnvironment(t, "DING_SELFHOST_TEST_DATABASE")
	fixture.SetMigrationEnvironment(t, "DING_SELFHOST_TEST_MIGRATION")
	dir := t.TempDir()
	config := filepath.Join(dir, "server.json")
	password := filepath.Join(dir, "password")
	body, err := json.Marshal(map[string]any{"publicURL": "https://ding.example", "dataDir": filepath.Join(dir, "state"), "keyFile": filepath.Join(dir, "key"), "database": database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "DING_SELFHOST_TEST_DATABASE"}, MaxOpenConns: 1}})
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{config: body, password: []byte("long initial password\n")} {
		f, err := mcpconfig.CreatePrivate(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(data); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	run := func(args ...string) error {
		var output bytes.Buffer
		if err := Execute("test", append([]string{"server", "--config", config}, args...), &output, &output); err != nil {
			return fmt.Errorf("%w: %s", err, output.String())
		}
		return nil
	}
	if err = run("run", "--check-config"); err != nil {
		t.Fatal(err)
	}
	if err = run("init", "--username", "operator", "--password-file", password); err == nil {
		t.Fatal("unmigrated PostgreSQL initialized silently")
	}
	if err = run("migrate", "--database-admin-env", "DING_SELFHOST_TEST_MIGRATION"); err != nil {
		t.Fatal(err)
	}
	if err = run("init", "--username", "operator", "--password-file", password); err != nil {
		t.Fatal(err)
	}
	if err = run("init", "--username", "other", "--password-file", password); err == nil {
		t.Fatal("second bootstrap succeeded")
	}
	if err = run("recover", "--username", "operator", "--password-file", password); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "state", "control.db")); !os.IsNotExist(err) {
		t.Fatal("PostgreSQL CLI created SQLite control state")
	}
	var output bytes.Buffer
	if err = Execute("test", []string{"--state-dir", filepath.Join(dir, "state"), "daemon"}, &output, &output); err == nil {
		t.Fatal("personal daemon opened PostgreSQL server artifacts")
	}
}
