package logs

import (
	"testing"
	"time"
)

func TestSummaryCannotOverrideAppliedPrerequisites(t *testing.T) {
	end := time.Unix(100, 0)
	c := WindowConfig{Window: time.Minute, MinEvents: 100}
	bogus := 1.0
	s := Summary{Version: 1, WindowSeconds: 60, WindowStart: end.Add(-time.Minute), WindowEnd: end, Total: 10, Classified: 10, Warnings: 3, Complete: true, Ready: true, WarningRatio: &bogus}
	got, err := s.Assess(c, end.Add(-2*time.Minute), end)
	if err != nil || got.Ready || got.WarningRatio != nil || got.Reason != "insufficient_records" {
		t.Fatal(got, err)
	}
	c.MinEvents = 1
	got, err = s.Assess(c, end.Add(-2*time.Minute), end)
	if err != nil || !got.Ready || *got.WarningRatio != 0.3 {
		t.Fatal(got, err)
	}
	got, err = s.Assess(c, end.Add(-time.Second), end)
	if err != nil || got.Ready || got.Reason != "warming_up" {
		t.Fatal(got, err)
	}
	if _, err := s.Assess(c, end.Add(-time.Hour), end.Add(4*time.Second)); err == nil {
		t.Fatal("accepted stale summary")
	}
	s.Total = 2
	if _, err := s.Assess(c, end.Add(-time.Hour), end); err == nil {
		t.Fatal("accepted impossible counts")
	}
}
