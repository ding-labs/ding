package update

import (
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/install"
)

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
