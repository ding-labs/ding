package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
)

func TestInvalidStateRequiresExplicitReset(t *testing.T) {
	e, err := evaluator.NewEngine(nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{`not json`, `{"version":1}`, `{"version":99}`} {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if err := restoreStateFile(e, path, time.Now()); err == nil {
			t.Fatal("bad state accepted")
		}
		if got, _ := os.ReadFile(path); string(got) != content {
			t.Fatal("bad state overwritten")
		}
	}
	if err := restoreStateFile(e, filepath.Join(t.TempDir(), "new.json"), time.Now()); err != nil {
		t.Fatal(err)
	}
}
