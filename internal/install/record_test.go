package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordPreservesExistingOwnerAndState(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(t.TempDir(), "ding")
	if err := os.WriteFile(exe, []byte("test executable"), 0700); err != nil {
		t.Fatal(err)
	}
	r, err := Inspect(exe, dir, "dev", "standalone")
	if err != nil {
		t.Fatal(err)
	}
	if err := Create(r); err != nil {
		t.Fatal(err)
	}
	if err := Create(r); err != nil {
		t.Fatal(err)
	}
	r.Owner = "homebrew"
	if err := Create(r); err == nil {
		t.Fatal("installation owner overwritten")
	}
	got, err := Load(dir)
	if err != nil || got.Owner != "standalone" || got.Channel != "development" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, RecordName), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Create(r); err == nil {
		t.Fatal("damaged record overwritten")
	}
}

func TestRecordRejectsWrongDirectoryAndUnsafePath(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "ding")
	if err := os.WriteFile(exe, []byte("test"), 0700); err != nil {
		t.Fatal(err)
	}
	r, err := Inspect(exe, t.TempDir(), "1.0.0-preview.1", "external")
	if err != nil {
		t.Fatal(err)
	}
	if err := Create(r); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(r.StateDir, RecordName))
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, RecordName), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(other); err == nil {
		t.Fatal("accepted foreign installation")
	}
	r.Executable += "\nother command"
	if err := r.Validate(); err == nil {
		t.Fatal("accepted unsafe executable")
	}
}
