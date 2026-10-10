package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/install"
)

func TestHomebrewDoesNotRequireStandaloneUpdateTrust(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(t.TempDir(), "ding")
	if err := os.WriteFile(exe, []byte("test"), 0700); err != nil {
		t.Fatal(err)
	}
	r, err := install.Inspect(exe, dir, "0.15.0", "homebrew")
	if err != nil {
		t.Fatal(err)
	}
	if err := install.Create(r); err != nil {
		t.Fatal(err)
	}
	status, err := CheckOnce(context.Background(), dir, 5, true)
	if err != nil || status.Error != "" || status.Current != "0.15.0" || !status.CheckedAt.IsZero() {
		t.Fatal(status, err)
	}
}

func TestUpdateSettingsFailClosed(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadSettings(dir)
	if err != nil || !s.Checks || s.Automatic || s.HourUTC != 3 {
		t.Fatal(s, err)
	}
	for _, invalid := range []Settings{
		{Checks: true, Automatic: true, HourUTC: 24},
		{Checks: true, HourUTC: -1},
		{Automatic: true, HourUTC: 3},
	} {
		if err := install.AtomicJSON(filepath.Join(dir, "updates.json"), invalid); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSettings(dir); err == nil {
			t.Fatal("accepted inconsistent update settings", invalid)
		}
	}
}
