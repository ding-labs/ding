package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineCommandsDoNotConstructRuntime(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "ding.yaml")
	alertLog := filepath.Join(dir, "alerts.json")
	data := "notifiers:\n  k8s: {type: kubernetes_event}\n  gl: {type: gitlab_artifact, path: " + filepath.Join(dir, "artifact.md") + "}\nalert_log: {path: " + alertLog + "}\nrules:\n- name: sample\n  condition: value > 1\n  alert: [{notifier: k8s}]\n"
	if err := os.WriteFile(cfg, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runValidate(cfg); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := runTestRule(cfg, "json", true, strings.NewReader("{\"metric\":\"m\",\"value\":2}\n"), &out, &stderr); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatal("runtime side effect", err, files)
	}
	data += "  guard: {url: 'http://127.0.0.1:1', expect_status: 200}\n"
	os.WriteFile(cfg, []byte(data), 0600)
	if err := runTestRule(cfg, "json", true, strings.NewReader(""), &out, &stderr); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatal(err)
	}
}
