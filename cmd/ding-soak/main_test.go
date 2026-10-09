package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestLatencyHistogramUpperBound(t *testing.T) {
	var h histogram
	for i := 0; i < 95; i++ {
		h.add(4200 * time.Microsecond)
	}
	for i := 0; i < 5; i++ {
		h.add(time.Second)
	}
	if h.p95() != 5 {
		t.Fatal(h.p95())
	}
	h.add(time.Hour)
	if h.Counts[10000] != 1 {
		t.Fatal("overflow not bounded")
	}
}

func TestContinuityRejectsSuspendAndClockSteps(t *testing.T) {
	for _, test := range []struct {
		name                 string
		wall, monotonic, gap time.Duration
		fail                 bool
	}{
		{"normal", time.Hour, time.Hour, time.Second, false},
		{"small adjustment", time.Hour + time.Millisecond, time.Hour, time.Second, false},
		{"VM suspension", 19 * time.Hour, 6 * time.Hour, time.Second, true},
		{"backwards wall clock", time.Hour - 5*time.Second, time.Hour, time.Second, true},
		{"process pause", time.Hour, time.Hour, 15 * time.Second, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := checkContinuity(test.wall, test.monotonic, test.gap)
			if (err != nil) != test.fail {
				t.Fatal(err)
			}
		})
	}
}

func TestPushDriverReadsCurrentGeneration(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := watchrun.New(s)
	_, err = a.Apply(ctx, watchrun.ApplyRequest{Manifest: `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: push}
spec:
  source: {type: push, fields: {value: value}}
  condition: {field: value, operator: gte, value: 1}
  policy: {trigger: level, interval: 0s}
`})
	if err != nil {
		t.Fatal(err)
	}
	old, err := a.Record(ctx, "push")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Reserve(); err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	body := []byte(`{"value":2}`)
	if err := ingestCurrent(ctx, a, body, "first"); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"pause", "resume"} {
		if _, err := a.Lifecycle(ctx, "push", watchrun.LifecycleRequest{Action: action}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.IngestReserved(ctx, old, body, "second"); !errors.Is(err, store.ErrStale) {
		t.Fatal("stale fixture must reproduce the rejection", err)
	}
	if err := ingestCurrent(ctx, a, body, "second"); err != nil {
		t.Fatal("driver reused stale generation", err)
	}
	if err := s.View(ctx, func(tx *store.Tx) error {
		events, err := tx.Events("push", 0, 100)
		if err != nil {
			return err
		}
		n := 0
		for _, e := range events {
			if e.Type == "firing" {
				n++
			}
		}
		if n != 2 {
			t.Fatalf("got %d firings, want two", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestMedian(t *testing.T) {
	if median([]int64{90, 10, 20}) != 20 {
		t.Fatal("median")
	}
}

func TestMemoryMeasuresHighWaterMark(t *testing.T) {
	rss, peak, err := parseMemory("Name:\tding\nVmHWM:\t40000 kB\nVmRSS:\t30000 kB\n")
	if err != nil || rss != 30000*1024 || peak != 40000*1024 {
		t.Fatal(rss, peak, err)
	}
	for _, bad := range []string{"", "VmRSS: 10 kB", "VmRSS: 20 kB\nVmHWM: 10 kB", "VmRSS: text kB\nVmHWM: 30 kB"} {
		if _, _, err := parseMemory(bad); err == nil {
			t.Fatal("invalid memory accepted", bad)
		}
	}
}
