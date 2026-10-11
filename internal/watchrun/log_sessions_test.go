package watchrun

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/store"
)

const nativeLogManifest = `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: output}
spec:
  source: {type: logs, logs: {window: 1s, minEvents: 2}}
  condition: {field: warning_match_ratio, operator: gt, value: 0.5}
`

func TestLogSessionsFenceProducersAndPreserveIncident(t *testing.T) {
	a, _ := setup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: nativeLogManifest}); err != nil {
		t.Fatal(err)
	}
	session, err := a.StartLogSession(ctx, "output")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartLogSession(ctx, "output"); !errors.Is(err, ErrLogSessionBusy) {
		t.Fatal(err)
	}
	now := start.Add(time.Second)
	a.Now = func() time.Time { return now }
	input := LogSnapshot{Session: session.ID, Sequence: 1, Summary: logs.Summary{Version: 1, WindowSeconds: 1, WindowStart: start, WindowEnd: now, Total: 2, Classified: 2, Warnings: 2, Complete: true}}
	first, err := a.AcceptLogSnapshot(ctx, "output", input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := a.AcceptLogSnapshot(ctx, "output", input)
	if err != nil || !duplicate.Duplicate || duplicate.First != first.First {
		t.Fatal(duplicate, err)
	}
	input.Sequence = 3
	if _, err := a.AcceptLogSnapshot(ctx, "output", input); !errors.Is(err, store.ErrStale) {
		t.Fatal(err)
	}
	input.Sequence, input.Summary.Total, input.Summary.Classified, input.Summary.Warnings = 2, 0, 0, 0
	if _, err := a.AcceptLogSnapshot(ctx, "output", input); !errors.Is(err, store.ErrStale) {
		t.Fatal("re-evaluated the same completed second", err)
	}
	now = now.Add(time.Second)
	input.Summary.WindowStart, input.Summary.WindowEnd = now.Add(-time.Second), now
	if _, err := a.AcceptLogSnapshot(ctx, "output", input); err != nil {
		t.Fatal(err)
	}
	i, _ := a.Inspect(ctx, "output")
	for _, raw := range i.Entities {
		var state condition.State
		json.Unmarshal(raw, &state)
		if !state.Open || state.SourceUnhealthy || state.WaitingReason != "insufficient_records" {
			t.Fatal(state)
		}
	}
	input.Sequence, input.Ended = 3, true
	if _, err := a.AcceptLogSnapshot(ctx, "output", input); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartLogSession(ctx, "output"); err != nil {
		t.Fatal(err)
	}
	input.Sequence = 4
	if _, err := a.AcceptLogSnapshot(ctx, "output", input); !errors.Is(err, store.ErrStale) {
		t.Fatal(err)
	}
}

func TestLogSessionsExpireAcrossPauseAndDaemonRestart(t *testing.T) {
	a, _ := setup(t)
	a.Apply(ctx, ApplyRequest{Manifest: nativeLogManifest})
	session, _ := a.StartLogSession(ctx, "output")
	a.Lifecycle(ctx, "output", LifecycleRequest{Action: "pause"})
	a.Lifecycle(ctx, "output", LifecycleRequest{Action: "resume"})
	if _, err := a.AcceptLogSnapshot(ctx, "output", LogSnapshot{Session: session.ID, Sequence: 1, Ended: true}); !errors.Is(err, store.ErrStale) {
		t.Fatal(err)
	}
	session, err := a.StartLogSession(ctx, "output")
	if err != nil {
		t.Fatal(err)
	}
	restarted := New(a.Store)
	restarted.Now = a.Now
	if _, err := restarted.AcceptLogSnapshot(ctx, "output", LogSnapshot{Session: session.ID, Sequence: 1, Ended: true}); !errors.Is(err, store.ErrStale) {
		t.Fatal(err)
	}
	records, _ := restarted.List(ctx)
	restarted.expireLogSessions(ctx, records, start)
	if _, err := restarted.StartLogSession(ctx, "output"); err != nil {
		t.Fatal(err)
	}
}
