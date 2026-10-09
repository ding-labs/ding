package watchcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyCommandsAndConfigGiveMigrationPath(t *testing.T) {
	for _, name := range []string{"run", "serve", "test-rule", "install"} {
		var out, errs bytes.Buffer
		err := Execute("preview", []string{name, "--config", "old.yaml", "--json"}, &out, &errs)
		if err == nil || !json.Valid(errs.Bytes()) || !strings.Contains(errs.String(), "legacy_command") || !strings.Contains(errs.String(), "v0.14.0") || out.Len() != 0 {
			t.Fatal(name, err, out.String(), errs.String())
		}
	}
	path := filepath.Join(t.TempDir(), "old.yaml")
	os.WriteFile(path, []byte("rules: [{name: old, condition: 'value > 0'}]"), 0600)
	var out, errs bytes.Buffer
	if err := Execute("preview", []string{"validate", path, "--json"}, &out, &errs); err == nil || !strings.Contains(errs.String(), "ding migrate") {
		t.Fatal(err, errs.String())
	}
	for _, args := range [][]string{{"version"}, {"version", "--json"}, {"--version"}, {"--help"}} {
		out.Reset()
		errs.Reset()
		if err := Execute("preview", args, &out, &errs); err != nil || out.Len() == 0 {
			t.Fatal(args, err, errs.String())
		}
		if args[0] == "--help" && strings.Contains(out.String(), "ding-watch") {
			t.Fatal("old binary in help")
		}
	}
}
