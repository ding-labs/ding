package install

import (
	"strings"
	"testing"
)

func TestInstallationLockSurvivesFileAndReleases(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Lock(dir); err == nil {
		other()
		t.Fatal("concurrent mutation admitted")
	} else if !strings.Contains(err.Error(), "another installation operation") {
		t.Fatal("installation lock diagnostic changed", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock, err = Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}
