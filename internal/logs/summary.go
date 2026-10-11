package logs

import (
	"encoding/json"
	"fmt"
	"time"
)

// Assess validates a producer's counters and recomputes ratios/readiness from
// the applied configuration. Producers cannot override threshold prerequisites.
func (s Summary) Assess(c WindowConfig, started, now time.Time) (Summary, error) {
	if s.Version != 1 || s.WindowSeconds != int(c.Window/time.Second) || !s.WindowStart.Equal(s.WindowEnd.Add(-c.Window)) || !s.WindowEnd.Equal(s.WindowEnd.Truncate(time.Second)) || s.WindowEnd.After(now.Add(time.Second)) || !now.Before(s.WindowEnd.Add(3*time.Second)) {
		return s, fmt.Errorf("invalid or stale log summary window")
	}
	for _, n := range []int64{s.Total, s.Classified, s.Unclassified, s.Warnings, s.Errors, s.Fatal, s.Invalid} {
		if n < 0 || n > 1<<53-1 {
			return s, fmt.Errorf("invalid log summary count")
		}
	}
	if s.Classified+s.Unclassified != s.Total || s.Warnings+s.Errors+s.Fatal > s.Classified || s.Invalid > s.Unclassified {
		return s, fmt.Errorf("inconsistent log summary counts")
	}
	s.WarningRatio, s.ErrorRatio, s.Ready, s.Coverage = nil, nil, false, 0
	if s.Total > 0 {
		s.Coverage = float64(s.Classified) / float64(s.Total)
	}
	switch {
	case s.Invalid > 0:
		s.Complete, s.Reason = false, "invalid_records"
	case s.WindowStart.Before(started):
		s.Complete, s.Reason = false, "warming_up"
	case !s.Complete:
		s.Reason = "collection_gap"
	case s.Total < int64(c.MinEvents):
		s.Reason = "insufficient_records"
	case s.Coverage < c.MinCoverage:
		s.Reason = "insufficient_coverage"
	default:
		s.Ready, s.Reason = true, "ready"
		warning := float64(s.Warnings) / float64(s.Total)
		errors := float64(s.Errors+s.Fatal) / float64(s.Total)
		s.WarningRatio, s.ErrorRatio = &warning, &errors
	}
	return s, nil
}

func (s Summary) Fields() (map[string]any, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	err = json.Unmarshal(data, &fields)
	return fields, err
}
