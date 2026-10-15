package database

import (
	"os"
	"testing"
)

func TestBackendAndInstanceBindingRefuseImplicitSwitch(t *testing.T) {
	dir := t.TempDir()
	if err := CheckDirectory(dir, "sqlite"); err != nil {
		t.Fatal(err)
	}
	if err := BindDirectory(dir, "postgres", "instance-a"); err != nil {
		t.Fatal(err)
	}
	if err := BindDirectory(dir, "postgres", "instance-a"); err != nil {
		t.Fatal(err)
	}
	if err := CheckDirectory(dir, "sqlite"); err == nil {
		t.Fatal("backend switch accepted")
	}
	if err := BindDirectory(dir, "postgres", "instance-b"); err == nil {
		t.Fatal("different database accepted")
	}
	info, err := os.Stat(dir + "/" + BindingFile)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("binding is not private", err)
	}
}
