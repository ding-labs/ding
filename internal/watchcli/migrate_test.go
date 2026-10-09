package watchcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationCLIReportsPartialConversionWithoutStartingDaemon(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "old.yaml")
	out := filepath.Join(dir, "converted")
	state := filepath.Join(dir, "unused-state")
	raw := []byte("rules:\n- name: supported\n  condition: value > 0\n  alert: [{notifier: stdout}]\n- name: old-run\n  mode: end-of-run\n  condition: avg(value) over run > 0\n")
	if err := os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Execute("test", []string{"migrate", "--config", config, "--out", out, "--state-dir", state, "--json"}, &stdout, &stderr)
	if err == nil || !json.Valid(stdout.Bytes()) || !json.Valid(stderr.Bytes()) {
		t.Fatal(err, stdout.String(), stderr.String())
	}
	files, err := os.ReadDir(out)
	if err != nil || len(files) != 2 {
		t.Fatal(files, err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("migration started runtime", err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := Execute("test", []string{"migrate", "--config", config, "--out", out, "--json"}, &stdout, &stderr); err == nil {
		t.Fatal("overwrote export")
	}
}
