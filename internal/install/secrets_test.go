package install

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrivateSecretsPreserveValuesAndRejectDamagedStore(t *testing.T) {
	dir := t.TempDir()
	if err := SetSecret(dir, "API_TOKEN", "sensitive one"); err != nil {
		t.Fatal(err)
	}
	if err := SetSecret(dir, "WEBHOOK", "sensitive two"); err != nil {
		t.Fatal(err)
	}
	values, err := ReadSecrets(dir)
	if err != nil || values["API_TOKEN"] != "sensitive one" || len(values) != 2 {
		t.Fatal("secret roundtrip failed", err)
	}
	if err := SetSecret(dir, "bad-name", "value"); err == nil {
		t.Fatal("accepted invalid reference")
	}
	path := filepath.Join(dir, "secrets.json")
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadSecrets(dir); err == nil {
			t.Fatal("read public secrets")
		}
		if err := os.Chmod(path, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SetSecret(dir, "OTHER", "value"); err == nil {
		t.Fatal("overwrote corrupt secret store")
	}
}
