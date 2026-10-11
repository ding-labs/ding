package logs

import (
	"testing"
	"time"
)

func TestWindowCountsAndExpiry(t *testing.T) {
	start := time.Unix(100, 0)
	w, err := NewWindow(WindowConfig{Window: 2 * time.Second, MinEvents: 2}, start)
	if err != nil {
		t.Fatal(err)
	}
	w.Add(start, Classification{Level: Warning})
	for range 3 {
		w.Add(start.Add(time.Second), Classification{Level: Info})
	}
	if s := w.Snapshot(start.Add(time.Second)); s.Ready || s.Reason != "warming_up" {
		t.Fatal(s)
	}
	s := w.Snapshot(start.Add(2 * time.Second))
	if !s.Ready || s.Total != 4 || *s.WarningRatio != 0.25 {
		t.Fatal("must divide summed counts, not average bucket ratios", s)
	}
	if s := w.Snapshot(start.Add(3 * time.Second)); !s.Ready || s.Total != 3 || *s.WarningRatio != 0 {
		t.Fatal(s)
	}
	if s := w.Snapshot(start.Add(4 * time.Second)); s.Ready || s.WarningRatio != nil || s.Reason != "insufficient_records" {
		t.Fatal("silence must age out records without inventing recovery", s)
	}
}

func TestWindowCoverageAndGaps(t *testing.T) {
	start := time.Unix(100, 0)
	w, _ := NewWindow(WindowConfig{Window: time.Second, MinEvents: 1, MinCoverage: 0.9}, start)
	w.Add(start, Classification{Level: Warning})
	w.Add(start, Classification{})
	s := w.Snapshot(start.Add(time.Second))
	if s.Ready || s.Coverage != 0.5 || s.Total != 2 || s.Reason != "insufficient_coverage" {
		t.Fatal(s)
	}
	w.Add(start.Add(time.Second), Classification{Invalid: true})
	if s := w.Snapshot(start.Add(2 * time.Second)); s.Complete || s.Reason != "invalid_records" {
		t.Fatal(s)
	}
	w.Gap(start.Add(2 * time.Second))
	if s := w.Snapshot(start.Add(2500 * time.Millisecond)); s.Complete || s.Reason != "collection_gap" {
		t.Fatal(s)
	}
	w.Add(start.Add(3*time.Second), Classification{Level: Error})
	if s := w.Snapshot(start.Add(4 * time.Second)); !s.Ready || *s.ErrorRatio != 1 {
		t.Fatal(s)
	}
}

func BenchmarkCountRecord(b *testing.B) {
	w, _ := NewWindow(WindowConfig{Window: time.Minute, MinEvents: 1}, time.Unix(100, 0))
	b.ReportAllocs()
	for b.Loop() {
		w.Add(time.Unix(100, 0), Classification{Level: Info})
	}
}
