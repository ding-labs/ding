package watchcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedServerBootstrapAndRecovery(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "server.json")
	password := filepath.Join(dir, "password")
	b, _ := json.Marshal(map[string]string{"publicURL": "https://ding.example", "dataDir": filepath.Join(dir, "state"), "keyFile": filepath.Join(dir, "key")})
	if err := os.WriteFile(config, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(password, []byte("long initial password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) error {
		t.Helper()
		var out bytes.Buffer
		return Execute("test", append([]string{"server", "--config", config}, args...), &out, &out)
	}
	if err := run("run", "--check-config"); err != nil {
		t.Fatal(err)
	}
	if err := run("init", "--username", "operator", "--password-file", password); err != nil {
		t.Fatal(err)
	}
	if err := run("init", "--username", "other", "--password-file", password); err == nil {
		t.Fatal("second bootstrap succeeded")
	}
	if err := run("recover", "--username", "operator", "--password-file", password); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(password, 0644); err != nil {
		t.Fatal(err)
	}
	if err := run("recover", "--username", "operator", "--password-file", password); err == nil {
		t.Fatal("public password accepted")
	}
	var out bytes.Buffer
	if err := Execute("test", []string{"--state-dir", filepath.Join(dir, "state"), "daemon"}, &out, &out); err == nil {
		t.Fatal("personal daemon opened shared state")
	}
}
