package pluginpackage

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemotePackages(t *testing.T) {
	for _, mode := range []string{"remote-chatgpt", "remote-claude"} {
		t.Run(mode, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "package")
			o := Options{Root: "../..", Mode: mode, Endpoint: "https://qualification.example/mcp", Output: out, Version: "0.1.0"}
			archive, err := Build(o)
			if err != nil {
				t.Fatal(err)
			}
			r, err := zip.OpenReader(archive)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			files := map[string]bool{}
			for _, f := range r.File {
				files[f.Name] = true
				if strings.HasPrefix(f.Name, "bin/") || strings.HasPrefix(f.Name, "hooks/") {
					t.Fatal(f.Name)
				}
			}
			config := ".mcp.json"
			manifest := ".claude-plugin/plugin.json"
			if mode == "remote-chatgpt" {
				config = "mcp.json"
				manifest = "plugin.json"
			}
			for _, path := range []string{manifest, config, "skills/ding-watch/SKILL.md", "skills/ding-watch/references/manifest.md", "skills/ding-setup/SKILL.md", "verification.md", "LICENSE"} {
				if !files[path] {
					t.Fatal("missing", path)
				}
			}
			f, err := r.Open(config)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(f)
			f.Close()
			if !strings.Contains(string(b), o.Endpoint) {
				t.Fatal("endpoint omitted")
			}
			b, _ = os.ReadFile(filepath.Join(out, "qualification.json"))
			var q map[string]any
			_ = json.Unmarshal(b, &q)
			if q["marketplaceApproved"] != false || q["signed"] != false {
				t.Fatal(q)
			}
			if _, err := Build(o); !os.IsExist(err) {
				t.Fatal("overwrote package", err)
			}
		})
	}
}

func TestUnsafeInputsCreateNoArtifact(t *testing.T) {
	for _, url := range []string{"http://localhost:7677/mcp", "https://user:secret@example.com/mcp", "https://example.com/mcp?token=secret", "https://example.com/mcp#fragment"} {
		out := filepath.Join(t.TempDir(), "artifact")
		_, err := Build(Options{Mode: "remote-chatgpt", Endpoint: url, Output: out, Version: "0.1.0"})
		if err == nil {
			t.Fatal(url)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("created artifact before validation")
		}
	}
	if err := validateRuntime("package.go", "darwin-arm64"); err == nil {
		t.Fatal("non-executable runtime accepted")
	}
}
