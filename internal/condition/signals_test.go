package condition_test

import (
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/watch"
)

func TestChangedBaselineTypesAndInterval(t *testing.T) {
	e := engine(t, func(d *watch.Definition) {
		d.Spec.Condition = watch.Condition{Field: "status", Operator: "changed"}
		d.Spec.Policy = watch.Policy{Trigger: "level", Interval: "10s"}
	})
	var s condition.State
	wantKinds(t, step(t, e, &s, obs(1, 0, false, "ok")))
	r := step(t, e, &s, obs(2, 5, true, "ok"))
	wantKinds(t, r, "changed")
	if len(r.Events[0].Evidence) != 2 || r.Events[0].Evidence[0] != 1 {
		t.Fatal("change lacks baseline evidence")
	}
	wantKinds(t, step(t, e, &s, obs(3, 10, "true", "ok"))) // interval suppresses delivery, baseline still changes type
	wantKinds(t, step(t, e, &s, obs(4, 15, "true", "ok")))
	wantKinds(t, step(t, e, &s, obs(5, 20, true, "ok")), "changed")
	wantKinds(t, step(t, e, &s, obs(6, 25, nil, "unknown")), "source_error")
	wantKinds(t, step(t, e, &s, obs(7, 30, nil, "ok")), "source_recovered", "changed")
	wantKinds(t, step(t, e, &s, obs(8, 35, nil, "ok")))
	if s.Open {
		t.Fatal("change opened incident")
	}
}
func TestNewEventDedupHorizonAndBudget(t *testing.T) {
	e := engine(t, func(d *watch.Definition) {
		d.Spec.Condition = watch.Condition{Field: "status", Operator: "new-event", DedupFor: "10s"}
		d.Spec.Policy = watch.Policy{Trigger: "level", Interval: "0s"}
		d.Spec.Limits.MaxSamples = 1
	})
	var s condition.State
	wantKinds(t, step(t, e, &s, obs(1, 0, "a", "ok")), "new-event")
	wantKinds(t, step(t, e, &s, obs(2, 5, "a", "ok")))
	r := step(t, e, &s, obs(3, 6, "b", "ok"))
	wantKinds(t, r, "source_error")
	if r.Reason != "dedup_budget" || len(s.Seen) != 1 {
		t.Fatal(r)
	}
	// Duplicate a at t=5 did not extend the horizon, so a is a new event at t=10.
	wantKinds(t, step(t, e, &s, obs(4, 10, "a", "ok")), "source_recovered", "new-event")
	if !s.Seen["a"].Equal(epoch.Add(20 * time.Second)) {
		t.Fatal(s.Seen)
	}
	r = step(t, e, &s, obs(5, 15, 5, "ok"))
	wantKinds(t, r, "source_error")
	if r.Known {
		t.Fatal("numeric unstable event ID accepted")
	}
}
