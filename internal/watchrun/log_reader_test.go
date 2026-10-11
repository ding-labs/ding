//go:build !windows

package watchrun

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ding-labs/ding/internal/condition"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDaemonFileReaderAcceptsThenStopsOnPause(t *testing.T) {
	a, _ := setup(t)
	a.Now = time.Now
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("INFO historical\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(nativeLogManifest, "window: 1s", fmt.Sprintf("input: {type: file, path: %q}, window: 1s", path), 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartLogSession(ctx, "output"); err == nil {
		t.Fatal("external session attached to a file reader")
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- a.Run(runCtx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	deadline := time.Now().Add(7 * time.Second)
	for {
		r, _ := a.Record(ctx, "output")
		var checkpoint logCheckpoint
		json.Unmarshal([]byte(r.Cursor), &checkpoint)
		if checkpoint.Position != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("file reader did not publish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for {
		file.WriteString("WARN live\nWARN live\n")
		i, _ := a.Inspect(ctx, "output")
		fired := false
		for _, raw := range i.Entities {
			var state condition.State
			json.Unmarshal(raw, &state)
			if state.Open {
				fired = true
			}
		}
		if fired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no firing event", i)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := a.Lifecycle(ctx, "output", LifecycleRequest{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	r, _ := a.Record(ctx, "output")
	file.WriteString("WARN after pause\n")
	time.Sleep(1100 * time.Millisecond)
	after, _ := a.Record(ctx, "output")
	if r.Cursor != after.Cursor {
		t.Fatal("paused reader committed another summary")
	}
}
