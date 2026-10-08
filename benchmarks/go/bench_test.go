// benchmarks/go/bench_test.go
package bench_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/ingester"
)

// BenchmarkProcessSimpleRule measures throughput of Engine.Process() with a
// single event-per-event threshold rule. Value is below threshold so no alert
// fires — measures pure evaluation cost, not notifier dispatch.
func BenchmarkProcessSimpleRule(b *testing.B) {
	engine, err := evaluator.NewEngine([]evaluator.EngineRule{
		{
			Name:      "high_cpu",
			Condition: "value > 95",
			Cooldown:  0,
			Alerts:    []string{},
		},
	}, 10000)
	if err != nil {
		b.Fatal(err)
	}
	event := ingester.Event{
		Metric: "cpu_usage",
		Value:  50.0, // below threshold: no alert fires, measures eval path only
		Labels: map[string]string{"host": "web-01"},
		At:     time.Now(),
	}
	now := time.Now()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.Process(event, now)
	}
}

// BenchmarkProcessWindowedRule measures throughput with a windowed rule.
// Each Process() call appends to a ring buffer then scans O(n) entries for the
// aggregate — this is the more expensive path.
func BenchmarkProcessWindowedRule(b *testing.B) {
	engine, err := evaluator.NewEngine([]evaluator.EngineRule{
		{
			Name:      "high_cpu_avg",
			Condition: "avg(value) over 5m > 80",
			Cooldown:  0,
			Alerts:    []string{},
		},
	}, 10000)
	if err != nil {
		b.Fatal(err)
	}
	event := ingester.Event{
		Metric: "cpu_usage",
		Value:  50.0, // below threshold
		Labels: map[string]string{"host": "web-01"},
		At:     time.Now(),
	}
	// Warm up ring buffer to 1000 entries before measuring
	warmNow := time.Now()
	for j := 0; j < 1000; j++ {
		warmEvent := ingester.Event{
			Metric: event.Metric,
			Value:  event.Value,
			Labels: event.Labels,
			At:     warmNow.Add(time.Duration(j-1000) * time.Millisecond),
		}
		engine.Process(warmEvent, warmNow)
	}
	for _, buffer := range evaluator.SnapshotEngine(engine).Buffers {
		if len(buffer.Entries) != 1000 {
			b.Fatalf("warmup retained %d entries", len(buffer.Entries))
		}
	}
	event.At = warmNow
	now := warmNow
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.Process(event, now)
	}
}

// BenchmarkProcess100Rules measures throughput when 100 rules are active —
// the full rule-scan loop overhead. Each event is evaluated against all 100
// rules; none fire (value below threshold).
func BenchmarkProcess100Rules(b *testing.B) {
	rules := make([]evaluator.EngineRule, 100)
	for i := range rules {
		rules[i] = evaluator.EngineRule{
			Name:      fmt.Sprintf("rule_%03d", i),
			Condition: "avg(value) over 5m > 80",
			Cooldown:  0,
			Alerts:    []string{},
		}
	}
	engine, err := evaluator.NewEngine(rules, 10000)
	if err != nil {
		b.Fatal(err)
	}
	event := ingester.Event{
		Metric: "cpu_usage",
		Value:  50.0,
		Labels: map[string]string{"host": "web-01"},
		At:     time.Now(),
	}
	// Warm up ring buffer to 1000 entries before measuring
	warmNow := time.Now()
	for j := 0; j < 1000; j++ {
		warmEvent := ingester.Event{
			Metric: event.Metric,
			Value:  event.Value,
			Labels: event.Labels,
			At:     warmNow.Add(time.Duration(j-1000) * time.Millisecond),
		}
		engine.Process(warmEvent, warmNow)
	}
	for _, buffer := range evaluator.SnapshotEngine(engine).Buffers {
		if len(buffer.Entries) != 1000 {
			b.Fatalf("warmup retained %d entries", len(buffer.Entries))
		}
	}
	event.At = warmNow
	now := warmNow
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.Process(event, now)
	}
}

// BenchmarkEngineInit measures the cost of creating a new Engine with 100 rules —
// this is the hot path during both cold start and hot-reload (new engine is built
// before the old one is swapped out).
func BenchmarkEngineInit(b *testing.B) {
	rules := make([]evaluator.EngineRule, 100)
	for i := range rules {
		rules[i] = evaluator.EngineRule{
			Name:      fmt.Sprintf("rule_%03d", i),
			Condition: "avg(value) over 5m > 80",
			Cooldown:  0,
			Alerts:    []string{},
		}
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := evaluator.NewEngine(rules, 10000)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEngineReinit measures the cost of building a replacement engine —
// the dominant cost in a hot-reload where a new engine is created from an
// existing rule set. The cost is dominated by NewEngine parsing all rules.
func BenchmarkEngineReinit(b *testing.B) {
	rules := make([]evaluator.EngineRule, 100)
	for i := range rules {
		rules[i] = evaluator.EngineRule{
			Name:      fmt.Sprintf("rule_%03d", i),
			Condition: "avg(value) over 5m > 80",
			Cooldown:  0,
			Alerts:    []string{},
		}
	}
	engine, err := evaluator.NewEngine(rules, 10000)
	if err != nil {
		b.Fatal(err)
	}
	_ = engine // original engine; replacement is a new one built each iteration
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		newEngine, err := evaluator.NewEngine(rules, 10000)
		if err != nil {
			b.Fatal(err)
		}
		_ = newEngine // in production, server.mu.Lock(); server.engine = newEngine
	}
}

func BenchmarkParseJSON(b *testing.B) {
	data := []byte(`{"metric":"cpu","value":12.5,"host":"a"}`)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ingester.ParseJSONLine(data); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkThousandGroups(b *testing.B) {
	e, err := evaluator.NewEngine([]evaluator.EngineRule{{Name: "groups", Condition: "avg(value) over 5m > 80"}}, 1000)
	if err != nil {
		b.Fatal(err)
	}
	now := time.Now()
	events := make([]ingester.Event, 1000)
	for i := range events {
		events[i] = ingester.Event{Metric: "cpu", Value: 1, Labels: map[string]string{"host": fmt.Sprint(i)}, At: now}
		e.Process(events[i], now)
	}
	if e.StateStats().LabelSets != 1000 {
		b.Fatal("cardinality warmup")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Process(events[i%1000], now)
	}
}
func BenchmarkSnapshotPersistence(b *testing.B) {
	e, _ := evaluator.NewEngine([]evaluator.EngineRule{{Name: "state", Condition: "avg(value) over 5m > 80"}}, 1000)
	now := time.Now()
	for i := 0; i < 1000; i++ {
		e.Process(ingester.Event{Value: 1, At: now}, now)
	}
	snap := evaluator.SnapshotEngine(e)
	path := b.TempDir() + "/state.json"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := evaluator.SaveSnapshot(path, snap); err != nil {
			b.Fatal(err)
		}
	}
}
