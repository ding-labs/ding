package replay

import (
	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
	"testing"
	"time"
)

func TestWindowExplanationUsesReplayedSamplesAndExclusiveBoundary(t *testing.T) {
	b, err := plan.Parse([]byte(`apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: window}
spec:
 source: {type: push}
 condition: {field: value, numeric: 'avg(value) over 1m > 5'}
 policy: {trigger: transition}
`))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	proof := Evidence{Definition: b.Watches[0].Definition, Checkpoint: &Checkpoint{Prior: condition.State{Entity: "[]", Samples: []condition.Sample{{At: now.Add(-time.Minute), Value: 100, Sequence: 1}, {At: now.Add(-time.Second), Value: 6, Sequence: 2}}}, Input: watch.Observation{Sequence: 3, AcceptedAt: now, Health: "ok", Fields: map[string]any{"value": 8}}}}
	e, err := Explain(proof)
	if err != nil || len(e.Windows) != 1 {
		t.Fatal(e, err)
	}
	w := e.Windows[0]
	if !w.Available || w.Samples != 2 || w.Value == nil || *w.Value != 7 || !e.Matched || !e.Open {
		t.Fatal(e, w)
	}
	proof.Checkpoint.Input.Health = "unknown"
	e, err = Explain(proof)
	if err != nil {
		t.Fatal(err)
	}
	if e.Known || e.Windows[0].Value != nil || e.Windows[0].Available {
		t.Fatal("invented aggregate for unknown evaluation", e)
	}
}
