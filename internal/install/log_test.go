package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackgroundLogIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	l, err := OpenLog(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := l.Write([]byte(strings.Repeat("x", 800))); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := l.Write([]byte(strings.Repeat("y", 4096))); err != nil || n != 4096 {
		t.Fatal(n, err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatal("unbounded rotation", len(entries))
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.Size() > 1024 {
			t.Fatal(info, err)
		}
	}
}
