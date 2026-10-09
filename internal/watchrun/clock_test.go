package watchrun

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func TestClockDiscontinuityDistinguishesSchedulerDelay(t *testing.T) {
	for _, tc := range []struct {
		wall, elapsed time.Duration
		jump          bool
	}{{time.Hour, time.Hour, false}, {100 * time.Millisecond, 100 * time.Millisecond, false}, {time.Hour, time.Second, true}, {-time.Hour, time.Second, true}, {1500 * time.Millisecond, time.Second, false}} {
		if clockDiscontinuity(start, start.Add(tc.wall), tc.elapsed) != tc.jump {
			t.Fatal(tc)
		}
	}
}
func TestClockShiftRearmsMissingDataAndFencesOldAcquisition(t *testing.T) {
	for _, direction := range []time.Duration{time.Hour, -time.Hour} {
		t.Run(direction.String(), func(t *testing.T) {
			a, _ := setup(t)
			m := strings.Replace(manifest("https://example.com"), "field: http.status, operator: gte, value: 500", "missingFor: 10s", 1)
			m = strings.Replace(m, "consecutive: 3, recoverAfter: 2", "consecutive: 1, recoverAfter: 1", 1)
			if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
				t.Fatal(err)
			}
			old, _ := a.Record(ctx, "api")
			now := start.Add(direction)
			if err := a.clockShift(ctx, now); err != nil {
				t.Fatal(err)
			}
			if err := a.RunTimers(ctx, now); err != nil {
				t.Fatal(err)
			}
			s := state(t, a)
			if s.Open || !s.SourceUnhealthy || !s.MissingAt.Equal(now.Add(10*time.Second)) {
				t.Fatal(s)
			}
			if _, err := a.Accept(ctx, old, source.Batch{Observations: []watch.Observation{{Health: "ok"}}}, "stale", now); !errors.Is(err, store.ErrStale) {
				t.Fatal(err)
			}
			if err := a.RunTimers(ctx, now.Add(10*time.Second)); err != nil {
				t.Fatal(err)
			}
			if !state(t, a).Open {
				t.Fatal("rearmed deadline did not fire")
			}
			var events []watch.Event
			a.Store.View(ctx, func(tx *store.Tx) error { events, _ = tx.Events("api", 0, 100); return nil })
			for _, event := range events {
				if _, err := a.Evidence(ctx, event.ID); err != nil {
					t.Fatal(event.Type, err)
				}
			}
		})
	}
}
func TestClockShiftHoldsWindowUnknownUntilNewTimeSpan(t *testing.T) {
	a, _ := setup(t)
	m := strings.Replace(manifest("https://example.com"), "field: http.status, operator: gte, value: 500", "field: http.status, numeric: 'avg(value) over 1m > 400'", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	r, _ := a.Record(ctx, "api")
	for n := 1; n <= 3; n++ {
		input(t, a, r, n, 500)
	}
	now := start.Add(time.Hour)
	if err := a.clockShift(ctx, now); err != nil {
		t.Fatal(err)
	}
	r, _ = a.Record(ctx, "api")
	accept := func(id string, at time.Time) {
		t.Helper()
		if _, err := a.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 200}}}}, id, at); err != nil {
			t.Fatal(err)
		}
	}
	accept("early", now.Add(time.Second))
	s := state(t, a)
	if !s.Open || !s.SourceUnhealthy || s.Recoveries != 0 {
		t.Fatal("clock jump invented recovery", s)
	}
	accept("fresh", now.Add(time.Minute))
	if s := state(t, a); !s.Open || s.SourceUnhealthy || s.Recoveries != 1 {
		t.Fatal(s)
	}
	accept("fresh2", now.Add(time.Minute+time.Second))
	if state(t, a).Open {
		t.Fatal("fresh recovery failed")
	}
}
