package bench_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/transform"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func BenchmarkWatchEvaluation(b *testing.B) {
	for _, numeric := range []string{"value > 95", "avg(value) over 1m > 95"} {
		b.Run(numeric, func(b *testing.B) {
			p, err := plan.Compile(watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: "bench"}, Spec: watch.Spec{Source: watch.Source{Type: "push"}, Condition: watch.Condition{Field: "value", Numeric: numeric}}})
			if err != nil {
				b.Fatal(err)
			}
			e, err := condition.New(p.Definition, p.Revision)
			if err != nil {
				b.Fatal(err)
			}
			now := time.Unix(1700000000, 0).UTC()
			state := condition.State{}
			sequence := int64(0)
			evaluate := func() {
				sequence++
				at := now.Add(time.Duration(sequence) * time.Second)
				r, err := e.Evaluate(state, watch.Observation{Sequence: sequence, AcceptedAt: at, Health: "ok", Fields: map[string]any{"value": 50.0}}, at)
				if err != nil || !r.Known {
					b.Fatalf("evaluation invalid: %s %v", r.Reason, err)
				}
				state = r.State
			}
			for n := 0; n < 60; n++ {
				evaluate()
			}
			if numeric != "value > 95" && len(state.Samples) != 60 {
				b.Fatal("invalid window warmup")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				evaluate()
			}
		})
	}
}
func BenchmarkPushCommit(b *testing.B) {
	ctx := context.Background()
	s, err := store.Open(ctx, b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	a := watchrun.New(s)
	m := `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: bench}
spec:
  source: {type: push}
  condition: {field: value, operator: gt, value: 95}
  policy: {trigger: level, interval: 0s}
`
	if _, err := a.Apply(ctx, watchrun.ApplyRequest{Manifest: m}); err != nil {
		b.Fatal(err)
	}
	r, err := a.Record(ctx, "bench")
	if err != nil {
		b.Fatal(err)
	}
	now := time.Now().UTC()
	batch := source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"value": 100.0}}}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := a.Accept(ctx, r, batch, strconv.Itoa(i), now.Add(time.Duration(i)*time.Millisecond)); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkJSONProjection(b *testing.B) {
	raw := []byte(`{"value":12.5,"host":"a"}`)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := transform.Project(context.Background(), raw, "", nil, 100, 1<<20); err != nil {
			b.Fatal(err)
		}
	}
}
