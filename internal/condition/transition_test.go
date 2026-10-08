package condition_test

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func definition(t *testing.T, edit func(*watch.Definition)) (watch.Definition, string) {
	t.Helper()
	d := watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: "api"}, Spec: watch.Spec{Source: watch.Source{Type: "http", URL: "https://example.com", Every: "5s"}, Condition: watch.Condition{Field: "status", Operator: "gte", Value: 500}, Policy: watch.Policy{Consecutive: 3, RecoverAfter: 2}}}
	if edit != nil {
		edit(&d)
	}
	p, err := plan.Compile(d)
	if err != nil {
		t.Fatal(err)
	}
	return p.Definition, p.Revision
}
func engine(t *testing.T, edit func(*watch.Definition)) *condition.Evaluator {
	t.Helper()
	d, r := definition(t, edit)
	e, err := condition.New(d, r)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func obs(seq int64, sec int, value any, health string) watch.Observation {
	return watch.Observation{Sequence: seq, AcceptedAt: epoch.Add(time.Duration(sec) * time.Second), Fields: map[string]any{"status": value}, Health: health}
}
func step(t *testing.T, e *condition.Evaluator, s *condition.State, o watch.Observation) condition.Transition {
	t.Helper()
	before, _ := json.Marshal(s)
	r, err := e.Evaluate(*s, o, o.AcceptedAt)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("mutated prior state")
	}
	*s = r.State
	return r
}
func kinds(r condition.Transition) []string {
	var out []string
	for _, e := range r.Events {
		out = append(out, e.Type)
	}
	return out
}
func wantKinds(t *testing.T, r condition.Transition, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(kinds(r), want) {
		t.Fatalf("events %v want %v (%s)", kinds(r), want, r.Reason)
	}
}

func TestIncidentAndReplayDeterminism(t *testing.T) {
	e := engine(t, nil)
	input := []watch.Observation{obs(1, 0, 500, "ok"), obs(2, 5, 502, "ok"), obs(3, 10, 503, "ok"), obs(4, 15, 500, "ok"), obs(5, 20, 200, "ok"), obs(6, 25, 200, "ok"), obs(7, 30, 500, "ok"), obs(8, 35, 500, "ok"), obs(9, 40, 500, "ok")}
	var first []condition.Transition
	for run := 0; run < 2; run++ {
		var s condition.State
		var got []condition.Transition
		for i, o := range input {
			r := step(t, e, &s, o)
			got = append(got, r)
			switch i {
			case 2, 8:
				wantKinds(t, r, "firing")
			case 5:
				wantKinds(t, r, "recovered")
			default:
				wantKinds(t, r)
			}
		}
		if run == 0 {
			first = got
		} else if !reflect.DeepEqual(first, got) {
			t.Fatal("replay differs")
		}
	}
	if !reflect.DeepEqual(first[2].Events[0].Evidence, []int64{1, 2, 3}) || !reflect.DeepEqual(first[5].Events[0].Evidence, []int64{5, 6}) {
		t.Fatal("incorrect evidence")
	}
	if first[2].Events[0].ID == first[8].Events[0].ID {
		t.Fatal("different incidents share identity")
	}
}
func TestUnknownGapAndUnchanged(t *testing.T) {
	e := engine(t, nil)
	var s condition.State
	step(t, e, &s, obs(1, 0, 500, "ok"))
	step(t, e, &s, obs(2, 5, 500, "ok"))
	wantKinds(t, step(t, e, &s, obs(3, 10, nil, "unknown")), "source_error")
	if s.Matches != 0 {
		t.Fatal("unknown retained streak")
	}
	wantKinds(t, step(t, e, &s, obs(4, 15, nil, "unknown")))
	wantKinds(t, step(t, e, &s, obs(5, 20, 500, "ok")), "source_recovered")
	wantKinds(t, step(t, e, &s, obs(6, 25, nil, "unchanged")))
	if s.Matches != 1 {
		t.Fatal("304 counted")
	}
	step(t, e, &s, obs(7, 30, 500, "ok"))
	wantKinds(t, step(t, e, &s, obs(8, 35, 500, "ok")), "firing")
	wantKinds(t, step(t, e, &s, obs(9, 40, nil, "unknown")), "source_error")
	if !s.Open {
		t.Fatal("unknown closed incident")
	}
	wantKinds(t, step(t, e, &s, obs(10, 45, 200, "ok")), "source_recovered")
	wantKinds(t, step(t, e, &s, obs(11, 50, nil, "gap")), "gap")
	if !s.Open || s.Recoveries != 0 {
		t.Fatal("gap mishandled")
	}
	wantKinds(t, step(t, e, &s, obs(12, 55, 200, "ok")))
	wantKinds(t, step(t, e, &s, obs(13, 60, 200, "ok")), "recovered")
	step(t, e, &s, obs(14, 65, 500, "ok"))
	wantKinds(t, step(t, e, &s, obs(15, 80, 500, "ok")), "gap")
	if s.Matches != 1 {
		t.Fatal("long gap retained streak")
	}
	wantKinds(t, step(t, e, &s, obs(16, 79, 500, "ok")), "source_error")
	if !s.LastAt.Equal(epoch.Add(80 * time.Second)) {
		t.Fatal("clock regression changed watermark")
	}
}
func TestLevelInterval(t *testing.T) {
	e := engine(t, func(d *watch.Definition) {
		d.Spec.Policy = watch.Policy{Trigger: "level", Interval: "10s", Consecutive: 1, RecoverAfter: 1}
	})
	var s condition.State
	for i, sec := range []int{0, 5, 10, 15, 20} {
		r := step(t, e, &s, obs(int64(i+1), sec, 500, "ok"))
		if sec%10 == 0 {
			wantKinds(t, r, "firing")
		} else {
			wantKinds(t, r)
		}
	}
	e = engine(t, func(d *watch.Definition) { d.Spec.Policy = watch.Policy{Trigger: "level", Interval: "0s"} })
	s = condition.State{}
	wantKinds(t, step(t, e, &s, obs(1, 0, 500, "ok")), "firing")
	wantKinds(t, step(t, e, &s, obs(2, 0, 500, "ok")), "firing")
}
func TestExactWindowsAndBudget(t *testing.T) {
	e := engine(t, func(d *watch.Definition) {
		d.Spec.Condition = watch.Condition{Field: "status", Numeric: "sum(value) over 10s > 5 AND count(value) over 10s >= 2"}
		d.Spec.Policy = watch.Policy{}
		d.Spec.Limits.MaxSamples = 2
	})
	var s condition.State
	wantKinds(t, step(t, e, &s, obs(1, 0, 3, "ok")))
	wantKinds(t, step(t, e, &s, obs(2, 5, 3, "ok")), "firing")
	r := step(t, e, &s, obs(3, 6, 3, "ok"))
	wantKinds(t, r, "source_error")
	if r.Known || len(s.Samples) != 2 {
		t.Fatal("overflow reported exact result")
	}
	// The t=0 sample is excluded at t=10, but the omitted t=6 sample still contaminates the window.
	r = step(t, e, &s, obs(4, 10, 3, "ok"))
	if r.Known || len(s.Samples) != 2 || s.Samples[0].Sequence != 2 {
		t.Fatal("window boundary or overflow recovery wrong")
	}
	r = step(t, e, &s, obs(5, 16, 3, "ok"))
	wantKinds(t, r, "source_recovered")
	if !r.Known || !r.Matched || !s.OverflowUntil.IsZero() {
		t.Fatal("did not regain exactness")
	}
	// Separate boundary proof: only the current observation remains at exactly +10s.
	e = engine(t, func(d *watch.Definition) {
		d.Spec.Condition = watch.Condition{Field: "status", Numeric: "count(value) over 10s >= 2"}
		d.Spec.Policy = watch.Policy{}
	})
	s = condition.State{}
	step(t, e, &s, obs(1, 0, 2, "ok"))
	r = step(t, e, &s, obs(2, 10, 2, "ok"))
	if r.Matched || len(s.Samples) != 1 {
		t.Fatal("left boundary included")
	}
}
func TestTypedComparisonsAndAggregates(t *testing.T) {
	for _, tc := range []struct {
		a            any
		op           string
		b            any
		known, match bool
	}{{5, "eq", 5.0, true, true}, {json.Number("6"), "gt", 5, true, true}, {5, "lt", 6, true, true}, {5, "lte", 5, true, true}, {5, "gte", 6, true, false}, {"5", "eq", 5, false, false}, {true, "eq", true, true, true}, {"up", "ne", "down", true, true}, {nil, "eq", nil, true, true}, {false, "eq", nil, true, false}, {nil, "ne", false, false, false}, {false, "gt", false, false, false}, {1, "wat", 1, false, false}} {
		k, m := condition.Compare(tc.a, tc.op, tc.b)
		if k != tc.known || m != tc.match {
			t.Errorf("%v %s %v => %v %v", tc.a, tc.op, tc.b, k, m)
		}
	}
	for _, v := range []any{math.Inf(1), math.NaN(), json.Number("no"), "5", nil} {
		if _, ok := condition.Number(v); ok {
			t.Errorf("accepted %v", v)
		}
	}
	if n, ok := condition.Number(int64(4)); !ok || n != 4 {
		t.Fatal("int64 rejected")
	}
	for fn, want := range map[string]float64{"sum": 12, "avg": 4, "min": 2, "max": 6, "count": 3} {
		if got := condition.Aggregate(fn, []float64{2, 4, 6}); got != want {
			t.Errorf("%s %v", fn, got)
		}
	}
	if !math.IsNaN(condition.Aggregate("bad", []float64{1})) || !math.IsNaN(condition.Aggregate("avg", nil)) {
		t.Fatal("invalid aggregate accepted")
	}
}
func TestGroupingFilteringAndImmutableEvents(t *testing.T) {
	e := engine(t, func(d *watch.Definition) {
		d.Spec.GroupBy = []string{"host"}
		d.Spec.Match = map[string]any{"service": "api"}
		d.Spec.Policy = watch.Policy{}
		d.Spec.Message = "{{.Watch}} {{.Type}} {{.Value}}"
	})
	var s condition.State
	o := obs(1, 0, 500, "ok")
	o.Fields["host"] = "a"
	o.Fields["service"] = "web"
	r := step(t, e, &s, o)
	if r.Reason != "filtered" {
		t.Fatal("match ignored")
	}
	o = obs(2, 5, 500, "ok")
	o.Fields["host"] = "a"
	o.Fields["service"] = "api"
	o.Fields["nested"] = map[string]any{"x": "before"}
	r = step(t, e, &s, o)
	wantKinds(t, r, "firing")
	if r.Events[0].Message != "api firing 500" {
		t.Fatal(r.Events[0].Message)
	}
	o.Fields["nested"].(map[string]any)["x"] = "after"
	if r.Events[0].Fields["nested"].(map[string]any)["x"] != "before" {
		t.Fatal("aliased event")
	}
	r = step(t, e, &s, obs(3, 10, nil, "unknown"))
	if r.Events[0].Entity != s.Entity {
		t.Fatal("source health changed entity")
	}
	o = obs(4, 15, 500, "ok")
	o.Fields["host"] = "b"
	if _, err := e.Evaluate(s, o, o.AcceptedAt); err == nil {
		t.Fatal("entity mix accepted")
	}
}
func TestValidationUnknownAndTemplateFailure(t *testing.T) {
	e := engine(t, nil)
	for _, o := range []watch.Observation{{}, obs(1, 0, 500, "ok")} {
		if _, err := e.Evaluate(condition.State{LastSequence: 1}, o, o.AcceptedAt); err == nil {
			t.Fatal("invalid sequence accepted")
		}
	}
	o := obs(1, 0, 500, "ok")
	if _, err := e.Evaluate(condition.State{}, o, o.AcceptedAt.Add(time.Second)); err == nil {
		t.Fatal("mismatched time accepted")
	}
	for _, value := range []any{"500", nil} {
		var s condition.State
		r := step(t, e, &s, obs(1, 0, value, "ok"))
		wantKinds(t, r, "source_error")
		if r.Known {
			t.Fatal("bad type known")
		}
	}
	var s condition.State
	o = obs(1, 0, 0, "ok")
	o.Fields = nil
	r := step(t, e, &s, o)
	if r.Reason != "missing_field" {
		t.Fatal(r.Reason)
	}
	e = engine(t, func(d *watch.Definition) { d.Spec.Policy = watch.Policy{}; d.Spec.Message = "{{.Fields.absent}}" })
	o = obs(1, 0, 500, "ok")
	if _, err := e.Evaluate(condition.State{}, o, o.AcceptedAt); err == nil {
		t.Fatal("missing template field silently accepted")
	}
	e = engine(t, func(d *watch.Definition) { d.Spec.Policy = watch.Policy{}; d.Spec.Message = "{{.Fields.long}}" })
	o.Fields["long"] = strings.Repeat("x", 65537)
	if _, err := e.Evaluate(condition.State{}, o, o.AcceptedAt); err == nil {
		t.Fatal("oversize message accepted")
	}
	d, rev := definition(t, nil)
	d.Spec.Policy.Consecutive = 0
	if _, err := condition.New(d, rev); err == nil {
		t.Fatal("uncompiled definition accepted")
	}
	d, rev = definition(t, nil)
	d.Spec.Condition.MissingFor = "1m"
	if _, err := condition.New(d, rev); err == nil {
		t.Fatal("unimplemented accepted")
	}
	d, rev = definition(t, nil)
	d.Spec.Condition.Numeric = "avg(value) over run > 1"
	if _, err := condition.New(d, rev); err == nil {
		t.Fatal("run window accepted")
	}
	d.Spec.Condition.Numeric = "oops"
	if _, err := condition.New(d, rev); err == nil {
		t.Fatal("invalid expression accepted")
	}
	d.Spec.Condition.Numeric = ""
	d.Spec.Message = "{{"
	if _, err := condition.New(d, rev); err == nil {
		t.Fatal("invalid template accepted")
	}
}
func TestWindowEvidenceAndOverflow(t *testing.T) {
	e := engine(t, func(d *watch.Definition) {
		d.Spec.Policy = watch.Policy{}
		d.Spec.Condition = watch.Condition{Field: "status", Numeric: "avg(value) over 10s > 1"}
	})
	var s condition.State
	step(t, e, &s, obs(1, 0, 0, "ok"))
	r := step(t, e, &s, obs(2, 5, 4, "ok"))
	if !reflect.DeepEqual(r.Events[0].Evidence, []int64{1, 2}) {
		t.Fatal("missing window evidence")
	}
	e = engine(t, func(d *watch.Definition) {
		d.Spec.Policy = watch.Policy{}
		d.Spec.Condition = watch.Condition{Field: "status", Numeric: "sum(value) over 10s > 1"}
	})
	s = condition.State{}
	step(t, e, &s, obs(1, 0, math.MaxFloat64, "ok"))
	r = step(t, e, &s, obs(2, 5, math.MaxFloat64, "ok"))
	if r.Reason != "numeric_overflow" || r.Known {
		t.Fatal("numeric overflow not visible")
	}
}
func FuzzEvaluationIsDeterministic(f *testing.F) {
	f.Add([]byte{5, 5, 0, 2, 3, 5})
	f.Fuzz(func(t *testing.T, values []byte) {
		if len(values) > 1000 {
			return
		}
		e := engine(t, nil)
		var a, b condition.State
		for i, v := range values {
			o := obs(int64(i+1), i, int(v), "ok")
			if v == 0 {
				o.Health = "unknown"
			}
			ra, ea := e.Evaluate(a, o, o.AcceptedAt)
			rb, eb := e.Evaluate(b, o, o.AcceptedAt)
			if ea != nil || eb != nil || !reflect.DeepEqual(ra, rb) {
				t.Fatal("nondeterminism")
			}
			a, b = ra.State, rb.State
		}
	})
}
