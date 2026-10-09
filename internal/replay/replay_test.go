package replay

import (
	"github.com/ding-labs/ding/internal/plan"
	"os"
	"strings"
	"testing"
)

func TestReplayFixture(t *testing.T) {
	data, err := os.ReadFile("../../examples/watches/api-health.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b, err := plan.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("../../testdata/watches/api-health.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := Run(b.Watches[0], f)
	if err != nil {
		t.Fatal(err)
	}
	if r.Observations != 5 || len(r.Events) != 2 || r.Events[0].Type != "firing" || r.Events[1].Type != "recovered" {
		t.Fatalf("%+v", r)
	}
	for _, fixture := range []string{`{`, `{"sequence":1,"acceptedAt":"2026-01-01T00:00:00Z","health":"ok","unexpected":1}`, `{} {}`, "{}"} {
		if _, err := Run(b.Watches[0], strings.NewReader(fixture)); err == nil {
			t.Errorf("accepted %q", fixture)
		}
	}
	b.Watches[0].Definition.Spec.Limits.MaxBytes = 32
	if _, err := Run(b.Watches[0], strings.NewReader(strings.Repeat("x", 100))); err == nil {
		t.Fatal("large record accepted")
	}
}

func TestGroupedHealthFanoutAndLimits(t *testing.T) {
	data, err := os.ReadFile("../../examples/watches/api-health.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b, err := plan.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	d := b.Watches[0].Definition
	d.Spec.GroupBy = []string{"host"}
	d.Spec.Policy.Consecutive = 1
	p, err := plan.Compile(d)
	if err != nil {
		t.Fatal(err)
	}
	input := `{"sequence":1,"acceptedAt":"2026-01-01T00:00:00Z","health":"ok","fields":{"host":"a","http.status":500}}
{"sequence":2,"acceptedAt":"2026-01-01T00:00:01Z","health":"ok","fields":{"host":"b","http.status":500}}
{"sequence":3,"acceptedAt":"2026-01-01T00:00:02Z","health":"unknown"}`
	r, err := Run(p, strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.States) != 2 || len(r.Events) != 4 {
		t.Fatalf("%+v", r)
	}
	if r.Events[2].Entity == r.Events[3].Entity {
		t.Fatal("fanout identity collision")
	}
	p.Definition.Spec.Limits.MaxEntities = 1
	if _, err := Run(p, strings.NewReader(input)); err == nil {
		t.Fatal("entity limit ignored")
	}
	p.Definition.Spec.Condition.Operator = "changed"
	if _, err := Run(p, strings.NewReader(input)); err == nil {
		t.Fatal("unsupported accepted")
	}
	p = b.Watches[0]
	input = `{"sequence":1,"health":"ok"}`
	if _, err := Run(p, strings.NewReader(input)); err == nil {
		t.Fatal("missing accepted time accepted")
	}
	p.Definition.Spec.GroupBy = []string{"host"}
	input = `{"sequence":1,"acceptedAt":"2026-01-01T00:00:00Z","health":"ok","fields":{"host":{}}}`
	if _, err := Run(p, strings.NewReader(input)); err == nil {
		t.Fatal("object grouping accepted")
	}
}

func TestMissingDataReplay(t *testing.T) {
	data, err := os.ReadFile("../../testdata/watches/missing-data.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := plan.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("../../testdata/watches/missing-data.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := Run(bundle.Watches[0], f)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"source_error", "firing", "source_recovered", "recovered"}
	if len(r.Events) != len(want) {
		t.Fatal(r.Events)
	}
	for i, event := range r.Events {
		if event.Type != want[i] {
			t.Fatal(r.Events)
		}
	}
	if len(r.Events[1].Evidence) != 2 || r.Events[1].Evidence[0] != 1 || r.Events[1].Evidence[1] != 3 {
		t.Fatal("absence lacks freshness/deadline evidence", r.Events[1])
	}
}
