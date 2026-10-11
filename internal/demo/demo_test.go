package demo

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestDemoCancellationCleansTemporaryState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := Run(ctx, Options{Progress: func(update Update) error {
		if update.Stage == "healthy" {
			cancel()
		}
		return nil
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary state leaked: %v, %v", entries, err)
	}
}

func TestDemoStreamsActualFiringAndRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("real scheduler")
	}
	var stages, events []string
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	err := Run(ctx, Options{Phase: 5 * time.Second, Progress: func(update Update) error {
		if update.Stage != "" {
			stages = append(stages, update.Stage)
		}
		if update.Event != nil {
			events = append(events, update.Event.Type)
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 3 || stages[1] != "failing" || len(events) != 2 || events[0] != "firing" || events[1] != "recovered" {
		t.Fatal(stages, events)
	}
}
