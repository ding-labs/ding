package logs

import (
	"fmt"
	"math"
	"sync"
	"time"
)

type WindowConfig struct {
	Window      time.Duration
	MinEvents   int
	MinCoverage float64
}

type bucket struct {
	second  int64
	counts  [7]int64
	invalid int64
}

// Window stores counters, never output text or one sample per record.
type Window struct {
	mu       sync.Mutex
	config   WindowConfig
	started  time.Time
	last     time.Time
	gapUntil time.Time
	buckets  []bucket
}

type Summary struct {
	Version       int       `json:"summary_version"`
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
	WindowSeconds int       `json:"window_seconds"`
	Total         int64     `json:"total_count"`
	Classified    int64     `json:"classified_count"`
	Unclassified  int64     `json:"unclassified_count"`
	Warnings      int64     `json:"warning_count"`
	Errors        int64     `json:"error_count"`
	Fatal         int64     `json:"fatal_count"`
	Invalid       int64     `json:"invalid_count"`
	Coverage      float64   `json:"classification_coverage"`
	WarningRatio  *float64  `json:"warning_match_ratio"`
	ErrorRatio    *float64  `json:"error_match_ratio"`
	Complete      bool      `json:"complete"`
	Ready         bool      `json:"ready"`
	Reason        string    `json:"reason"`
}

func NewWindow(c WindowConfig, at time.Time) (*Window, error) {
	if c.Window < time.Second || c.Window > time.Hour || c.Window%time.Second != 0 || c.MinEvents < 1 || math.IsNaN(c.MinCoverage) || c.MinCoverage < 0 || c.MinCoverage > 1 {
		return nil, fmt.Errorf("log windows require 1–3600 whole seconds, positive minimum records, and coverage between 0 and 1")
	}
	return &Window{config: c, started: at, last: at, buckets: make([]bucket, int(c.Window/time.Second)+1)}, nil
}

// Gap invalidates evidence without inventing the number or severity of lost logs.
func (w *Window) Gap(at time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gap(at)
}

// Reset starts a new live window after a collector session is replaced. Prior
// counts cannot establish continuity for the new session.
func (w *Window) Reset(at time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	clear(w.buckets)
	w.started, w.last, w.gapUntil = at, at, time.Time{}
}

func (w *Window) gap(at time.Time) {
	until := at.Add(w.config.Window)
	if until.After(w.gapUntil) {
		w.gapUntil = until
	}
}

func (w *Window) Add(at time.Time, c Classification) {
	if c.Blank {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	// Separate stdout/stderr readers can enqueue nearby records out of order.
	// Reject only records too old to fit the current bounded ring. The collector
	// reports actual clock discontinuities explicitly through Gap.
	if at.Before(w.last.Add(-w.config.Window)) {
		w.gap(w.last)
		return
	}
	if at.After(w.last) {
		w.last = at
	}
	second := at.Unix()
	index := (second%int64(len(w.buckets)) + int64(len(w.buckets))) % int64(len(w.buckets))
	b := &w.buckets[index]
	if b.second != second {
		*b = bucket{second: second}
	}
	if c.Level < Unclassified || c.Level > Fatal {
		c = Classification{Invalid: true}
	}
	// Keep integers exact when projected through JSON float64 fields.
	if b.counts[c.Level] >= (1<<53)/int64(len(w.buckets))/7 {
		w.gap(at)
		return
	}
	b.counts[c.Level]++
	if c.Invalid {
		b.invalid++
	}
}

// Snapshot counts completed second-aligned buckets in [end-window, end).
func (w *Window) Snapshot(at time.Time) Summary {
	w.mu.Lock()
	defer w.mu.Unlock()
	end := at.UTC().Truncate(time.Second)
	s := Summary{Version: 1, WindowStart: end.Add(-w.config.Window), WindowEnd: end, WindowSeconds: int(w.config.Window / time.Second), Reason: "ready"}
	var counts [7]int64
	for _, b := range w.buckets {
		if b.second < s.WindowStart.Unix() || b.second >= end.Unix() {
			continue
		}
		for i, n := range b.counts {
			counts[i] += n
			s.Total += n
		}
		s.Invalid += b.invalid
	}
	s.Unclassified, s.Warnings, s.Errors, s.Fatal = counts[Unclassified], counts[Warning], counts[Error], counts[Fatal]
	s.Classified = s.Total - s.Unclassified
	if s.Total > 0 {
		s.Coverage = float64(s.Classified) / float64(s.Total)
	}
	s.Complete = !end.Before(w.started.Add(w.config.Window)) && !end.Before(w.gapUntil) && s.Invalid == 0
	switch {
	case end.Before(w.gapUntil):
		s.Reason = "collection_gap"
	case s.Invalid > 0:
		s.Reason = "invalid_records"
	case end.Before(w.started.Add(w.config.Window)):
		s.Reason = "warming_up"
	case s.Total < int64(w.config.MinEvents):
		s.Reason = "insufficient_records"
	case s.Coverage < w.config.MinCoverage:
		s.Reason = "insufficient_coverage"
	default:
		s.Ready = true
		warning := float64(s.Warnings) / float64(s.Total)
		errors := float64(s.Errors+s.Fatal) / float64(s.Total)
		s.WarningRatio, s.ErrorRatio = &warning, &errors
	}
	return s
}
