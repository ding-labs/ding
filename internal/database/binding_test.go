package database

import (
	"github.com/ding-labs/ding/internal/mcpconfig"
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
	if err := mcpconfig.CheckPrivate(dir + "/" + BindingFile); err != nil {
		t.Fatal(err)
	}
}
