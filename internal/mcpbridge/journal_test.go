package mcpbridge

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ding-labs/ding/internal/mcpcontract"
)

func TestJournalBindsKeysAcrossTargetsAndRestarts(t *testing.T) {
	dir := t.TempDir()
	target := mcpcontract.Target{Connection: "local", Instance: "a", Workspace: "a", Execution: "local"}
	args := map[string]any{"watch_id": "same-name", "operation_key": "key"}
	for range 2 {
		if err := bind(dir, "key", "pause", args, target, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := bind(dir, "key", "receipt", nil, target, false); err != nil {
		t.Fatal(err)
	}
	other := target
	other.Connection = "cloud"
	for _, write := range []bool{true, false} {
		if err := bind(dir, "key", "pause", args, other, write); err == nil {
			t.Fatal("cross-target operation")
		}
	}
	other = target
	other.Instance = "replaced"
	if err := bind(dir, "key", "pause", args, other, true); err == nil {
		t.Fatal("instance replacement ignored")
	}
	if err := bind(dir, "key", "delete", args, target, true); err == nil {
		t.Fatal("operation changed")
	}
	// Independent processes also arbitrate via the private file lock.
	var successes atomic.Int32
	var wg sync.WaitGroup
	for _, target := range []mcpcontract.Target{target, other} {
		wg.Go(func() {
			if bind(dir, "race", "pause", args, target, true) == nil {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("conflicting writes admitted", successes.Load())
	}
}
