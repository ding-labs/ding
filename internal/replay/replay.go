// Package replay evaluates recorded observations without clocks, secrets, or I/O
// beyond the supplied reader. It never sends notifications.
package replay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

type Report struct {
	WatchID      string                     `json:"watchId"`
	Revision     string                     `json:"revision"`
	Observations int                        `json:"observations"`
	Events       []watch.Event              `json:"events"`
	States       map[string]condition.State `json:"states"`
}

func Run(p plan.Compiled, input io.Reader) (Report, error) {
	report := Report{WatchID: p.Definition.Metadata.ID, Revision: p.Revision, Events: []watch.Event{}, States: map[string]condition.State{}}
	e, err := condition.New(p.Definition, p.Revision)
	if err != nil {
		return report, err
	}
	scan := bufio.NewScanner(input)
	scan.Buffer(make([]byte, 4096), p.Definition.Spec.Limits.MaxBytes+1)
	var previous int64
	for scan.Scan() {
		if len(bytes.TrimSpace(scan.Bytes())) == 0 {
			continue
		}
		if report.Observations >= 100000 {
			return report, fmt.Errorf("fixture exceeds 100000 observations")
		}
		var o watch.Observation
		decoder := json.NewDecoder(bytes.NewReader(scan.Bytes()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&o); err != nil {
			return report, fmt.Errorf("observation %d: invalid JSON: %w", report.Observations+1, err)
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return report, fmt.Errorf("observation must be one JSON object")
		}
		if o.Sequence <= previous {
			return report, fmt.Errorf("observation sequence must increase across the fixture")
		}
		previous = o.Sequence
		keys := []string{}
		if o.Health != "ok" && len(report.States) > 0 {
			for key := range report.States {
				keys = append(keys, key)
			}
			sort.Strings(keys)
		} else {
			key, err := watch.EntityKey(p.Definition.Spec.GroupBy, o.Fields)
			if err != nil {
				return report, err
			}
			keys = append(keys, key)
		}
		for _, key := range keys {
			if _, exists := report.States[key]; !exists && len(report.States) >= p.Definition.Spec.Limits.MaxEntities {
				return report, fmt.Errorf("entity budget exceeded")
			}
			r, err := e.Evaluate(report.States[key], o, o.AcceptedAt)
			if err != nil {
				return report, fmt.Errorf("observation %d: %w", report.Observations+1, err)
			}
			report.States[key] = r.State
			report.Events = append(report.Events, r.Events...)
		}
		report.Observations++
	}
	if err := scan.Err(); err != nil {
		return report, fmt.Errorf("reading fixture: %w", err)
	}
	return report, nil
}
