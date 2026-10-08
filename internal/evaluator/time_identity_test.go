package evaluator

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/ingester"
)

func TestCooldownLogicalClockAndBoundary(t *testing.T) {
	now := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewCooldownTracker()
	if !c.TryAcquire("r", "x", time.Minute, now) {
		t.Fatal("first reservation failed")
	}
	if c.TryAcquire("r", "x", time.Minute, now.Add(59*time.Second)) {
		t.Fatal("early duplicate")
	}
	if got := c.RemainingString("r", "x", now.Add(30*time.Second)); got != "30s remaining" {
		t.Fatal(got)
	}
	if !c.TryAcquire("r", "x", time.Minute, now.Add(time.Minute)) {
		t.Fatal("exact expiry suppressed")
	}
	if !c.TryAcquire("r", "y", time.Minute, now) {
		t.Fatal("independent group suppressed")
	}
	if got := c.RemainingString("r", "x", now.Add(2*time.Minute)); got != "ready" {
		t.Fatal(got)
	}
	for i := 0; i < 2; i++ {
		if !c.TryAcquire("zero", "x", 0, now) {
			t.Fatal("zero cooldown suppressed")
		}
	}
	if c.TryAcquire("a:b", "c", time.Minute, now) == false || c.TryAcquire("a", "b:c", time.Minute, now) == false {
		t.Fatal("rule/group collision")
	}
}

func TestCooldownConcurrentReservation(t *testing.T) {
	c := NewCooldownTracker()
	now := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var acquired atomic.Int64
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if c.TryAcquire("r", "same", time.Minute, now) {
				acquired.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if n := acquired.Load(); n != 1 {
		t.Fatalf("got %d reservations; want exactly one", n)
	}
}

func TestEngineReplaysCooldownAndReportsExplicitTime(t *testing.T) {
	e, err := NewEngine([]EngineRule{{Name: "watch", Condition: "value > 0", Cooldown: time.Minute}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := ingester.Event{Metric: "price", Value: 1, At: now}
	for _, offset := range []time.Duration{0, 2 * time.Minute} {
		ev.At = now.Add(offset)
		if n := len(e.Process(ev, ev.At)); n != 1 {
			t.Fatalf("at %s: got %d alerts", offset, n)
		}
	}
	status := e.RulesStatus(now.Add(150 * time.Second))
	if got := status[0].CoolingDown[""]; got != "30s remaining" {
		t.Fatal(got)
	}
}

func TestDistinctLabelsHaveIndependentCooldowns(t *testing.T) {
	e, err := NewEngine([]EngineRule{{Name: "watch", Condition: "value > 0", Cooldown: time.Minute}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	labels := []map[string]string{{"a": "x,b=y"}, {"a": "x", "b": "y"}, {"a": ""}, {"": "a"}, nil}
	seen := map[string]bool{}
	for _, ls := range labels {
		key := LabelSetKey(ls)
		if seen[key] {
			t.Fatalf("colliding labels: %v", ls)
		}
		seen[key] = true
		if !reflect.DeepEqual(parseLabelKey(key), ls) {
			t.Fatalf("labels do not roundtrip: %v", ls)
		}
		if n := len(e.Process(ingester.Event{Metric: "m", Value: 1, Labels: ls, At: now}, now)); n != 1 {
			t.Fatalf("group %v suppressed", ls)
		}
	}
	if LabelSetKey(map[string]string{"z": "2", "a": "1"}) != LabelSetKey(map[string]string{"a": "1", "z": "2"}) {
		t.Fatal("map order changed key")
	}
}

func FuzzLabelIdentity(f *testing.F) {
	f.Add("a,b=x", "世界")
	f.Fuzz(func(t *testing.T, k, v string) {
		labels := map[string]string{k: v}
		got := parseLabelKey(LabelSetKey(labels))
		if !reflect.DeepEqual(got, labels) {
			t.Fatalf("roundtrip lost bytes: %#v", labels)
		}
	})
}
