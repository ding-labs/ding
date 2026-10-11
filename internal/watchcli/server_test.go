package watchcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

func TestSharedServerBootstrapAndRecovery(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "server.json")
	password := filepath.Join(dir, "password")
	b, _ := json.Marshal(map[string]string{"publicURL": "https://ding.example", "dataDir": filepath.Join(dir, "state"), "keyFile": filepath.Join(dir, "key")})
	for path, data := range map[string][]byte{config: b, password: []byte("long initial password\n")} {
		f, err := mcpconfig.CreatePrivate(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) error {
		t.Helper()
		var out bytes.Buffer
		if err := Execute("test", append([]string{"server", "--config", config}, args...), &out, &out); err != nil {
			return fmt.Errorf("%w: %s", err, out.String())
		}
		return nil
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
	// Windows privacy is enforced by ACLs, tested in mcpconfig; chmod does not
	// grant another Windows principal read access.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(password, 0644); err != nil {
			t.Fatal(err)
		}
		if err := run("recover", "--username", "operator", "--password-file", password); err == nil {
			t.Fatal("public password accepted")
		}
	}
	var out bytes.Buffer
	if err := Execute("test", []string{"--state-dir", filepath.Join(dir, "state"), "daemon"}, &out, &out); err == nil {
		t.Fatal("personal daemon opened shared state")
	}
}
