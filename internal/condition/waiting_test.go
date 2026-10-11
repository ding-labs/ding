package condition_test

import (
	"testing"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/watch"
)

func TestWaitingHoldsIncidentAndBreaksStreaks(t *testing.T) {
	e := engine(t, func(d *watch.Definition) { d.Spec.Policy.Consecutive = 1 })
	s := condition.State{}
	step(t, e, &s, obs(1, 1, 503, "ok"))
	step(t, e, &s, obs(2, 2, 200, "ok"))
	o := obs(3, 3, nil, "waiting")
	o.Detail = "insufficient_records"
	r := step(t, e, &s, o)
	if !s.Open || s.Recoveries != 0 || s.Matches != 0 || s.SourceUnhealthy || s.WaitingReason != o.Detail || len(r.Events) != 0 {
		t.Fatal(s, r)
	}
	step(t, e, &s, obs(4, 4, 200, "ok"))
	if !s.Open || s.WaitingReason != "" {
		t.Fatal("waiting was incorrectly counted toward recovery", s)
	}
	step(t, e, &s, obs(5, 5, 200, "ok"))
	if s.Open {
		t.Fatal("two ready false observations must recover")
	}
}

func TestWaitingRecoversSourceWithoutRecoveringIncident(t *testing.T) {
	e := engine(t, func(d *watch.Definition) { d.Spec.Policy.Consecutive = 1 })
	s := condition.State{}
	step(t, e, &s, obs(1, 1, 503, "ok"))
	step(t, e, &s, obs(2, 2, nil, "unknown"))
	r := step(t, e, &s, obs(3, 3, nil, "waiting"))
	if s.SourceUnhealthy || !s.Open || len(r.Events) != 1 || r.Events[0].Type != "source_recovered" {
		t.Fatal(s, r)
	}
}
