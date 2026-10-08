package evaluator

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/identity"
	"github.com/ding-labs/ding/internal/ingester"
)

func limitedEngine(t *testing.T, r EngineRule, max int, ttl time.Duration) *Engine {
	t.Helper()
	e, err := NewEngineWithLimits([]EngineRule{r}, 100, StateLimits{MaxLabelSets: max, IdleTTL: ttl})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestRestoreChangedWindowResetsInsteadOfReusingOldBuffer(t *testing.T) {
	now := time.Now()
	r := EngineRule{Name: "watch", Condition: "avg(value) over 1h > 40"}
	old := limitedEngine(t, r, 10, time.Hour)
	old.Process(ingester.Event{Metric: "m", Value: 100, At: now.Add(-10 * time.Minute)}, now.Add(-10*time.Minute))
	r.Condition = "avg(value) over 1m > 40"
	next := limitedEngine(t, r, 10, time.Hour)
	report, err := RestoreEngine(next, SnapshotEngine(old), now)
	if err != nil || !reflect.DeepEqual(report.ResetRules, []string{"watch"}) {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if got := next.Process(ingester.Event{Metric: "m", Value: 0, At: now}, now); len(got) != 0 {
		t.Fatalf("old window caused alert: %+v", got)
	}
}
func TestRestorePreservesPresentationChangesAndSeenCooldowns(t *testing.T) {
	now := time.Now()
	r := EngineRule{Name: "watch", Condition: "value > 0", Cooldown: time.Hour}
	old := limitedEngine(t, r, 10, time.Second)
	old.Process(ingester.Event{Metric: "m", Value: 1, Labels: map[string]string{"a": "b"}, At: now}, now)
	r.Message = "changed"
	r.Alerts = []string{"different"}
	next := limitedEngine(t, r, 10, time.Second)
	report, err := RestoreEngine(next, SnapshotEngine(old), now.Add(time.Minute))
	if err != nil || len(report.ResetRules) != 0 {
		t.Fatalf("%+v %v", report, err)
	}
	statuses := next.RulesStatus(now.Add(time.Minute))
	if got := statuses[0].CoolingDown[LabelSetKey(map[string]string{"a": "b"})]; got != "59m0s remaining" {
		t.Fatal(got)
	}
	next.Sweep(now.Add(2 * time.Minute))
	if next.StateStats().LabelSets != 1 {
		t.Fatal("active cooldown evicted")
	}
	next.Sweep(now.Add(2 * time.Hour))
	if next.StateStats().LabelSets != 0 || len(SnapshotEngine(next).Cooldowns) != 0 {
		t.Fatal("expired cooldown retained")
	}
}
func TestRestoreRemovedRuleAndMalformedState(t *testing.T) {
	now := time.Now()
	r := EngineRule{Name: "old", Condition: "avg(value) over 1m > 0"}
	old := limitedEngine(t, r, 10, time.Hour)
	old.Process(ingester.Event{Value: 1, At: now}, now)
	snap := SnapshotEngine(old)
	r.Name = "new"
	next := limitedEngine(t, r, 10, time.Hour)
	report, err := RestoreEngine(next, snap, now)
	if err != nil || len(report.ResetRules) != 1 || len(SnapshotEngine(next).Buffers) != 0 {
		t.Fatalf("%+v %v", report, err)
	}
	for _, change := range []func(*StateSnapshot){
		func(s *StateSnapshot) { s.Version = 99 },
		func(s *StateSnapshot) { s.Fingerprints = nil },
		func(s *StateSnapshot) { s.Buffers["broken"] = BufferSnapshot{} },
		func(s *StateSnapshot) {
			for k, b := range s.Buffers {
				b.Window = time.Hour
				s.Buffers[k] = b
			}
		},
		func(s *StateSnapshot) { s.Cooldowns["broken"] = now },
	} {
		s := SnapshotEngine(old)
		change(&s)
		before := SnapshotEngine(old)
		if _, err := RestoreEngine(old, s, now); err == nil {
			t.Fatal("malformed state accepted")
		}
		after := SnapshotEngine(old)
		if !reflect.DeepEqual(before.Buffers, after.Buffers) || !reflect.DeepEqual(before.Seen, after.Seen) {
			t.Fatal("failed restore mutated engine")
		}
	}
}
func TestRetentionExpiresInactiveGroups(t *testing.T) {
	now := time.Now()
	e := limitedEngine(t, EngineRule{Name: "r", Condition: "avg(value) over 1m > 80"}, 1100, time.Hour)
	for i := 0; i < 1000; i++ {
		e.Process(ingester.Event{Value: 50, At: now, Labels: map[string]string{"request": fmt.Sprint(i)}}, now)
	}
	later := now.Add(24 * time.Hour)
	e.Process(ingester.Event{Value: 50, At: later, Labels: map[string]string{"request": "new"}}, later)
	if got := e.StateStats(); got.LabelSets != 1 || got.Buffers != 1 {
		t.Fatalf("unbounded stale state: %+v", got)
	}
}
func TestQuotaPreservesActiveWindowsAndRunState(t *testing.T) {
	for _, condition := range []string{"avg(value) over 1h > 80", "avg(value) over run > 80"} {
		t.Run(condition, func(t *testing.T) {
			now := time.Now()
			e := limitedEngine(t, EngineRule{Name: "r", Condition: condition}, 1, time.Second)
			ev := ingester.Event{Value: 50, At: now, Labels: map[string]string{"host": "a"}}
			if _, err := e.ProcessChecked(ev, now); err != nil {
				t.Fatal(err)
			}
			later := now.Add(5 * time.Minute)
			ev.Labels = map[string]string{"host": "b"}
			ev.At = later
			if _, err := e.ProcessChecked(ev, later); err == nil {
				t.Fatal("quota silently exceeded")
			}
			if got := e.StateStats(); got.LabelSets != 1 || got.Buffers != 1 || got.Rejected != 1 {
				t.Fatalf("%+v", got)
			}
			e.Sweep(now.Add(24 * time.Hour))
			want := 0
			if condition == "avg(value) over run > 80" {
				want = 1
			}
			if got := e.StateStats().LabelSets; got != want {
				t.Fatalf("got %d want %d", got, want)
			}
		})
	}
}
func TestRestoreRespectsCurrentStateQuota(t *testing.T) {
	now := time.Now()
	r := EngineRule{Name: "r", Condition: "value > 0", Cooldown: time.Hour}
	old := limitedEngine(t, r, 2, time.Minute)
	for _, host := range []string{"a", "b"} {
		old.Process(ingester.Event{Value: 1, At: now, Labels: map[string]string{"host": host}}, now)
	}
	next := limitedEngine(t, r, 1, time.Minute)
	if _, err := RestoreEngine(next, SnapshotEngine(old), now); err == nil {
		t.Fatal("restore exceeded quota")
	}
	if next.StateStats().LabelSets != 0 {
		t.Fatal("failed restore published partial state")
	}
}
func TestSnapshotWithConcurrentEvaluationAndCleanup(t *testing.T) {
	now := time.Now()
	e := limitedEngine(t, EngineRule{Name: "r", Condition: "avg(value) over 1m > 0", Cooldown: time.Hour}, 100, time.Hour)
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				e.Process(ingester.Event{Value: 1, At: now}, now)
				e.Sweep(now)
				snap := SnapshotEngine(e)
				if len(snap.Cooldowns) > 0 && len(snap.Buffers) != 1 {
					t.Error("incoherent snapshot")
				}
			}
		}()
	}
	wg.Wait()
}
func TestRejectOldSnapshotPreservesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data := []byte(`{"version":1,"buffers":{}}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSnapshot(path); err == nil {
		t.Fatal("legacy snapshot silently accepted")
	}
	if got, _ := os.ReadFile(path); string(got) != string(data) {
		t.Fatal("old state changed")
	}
}
func TestIdentityMetadataRejectsMalformedLabels(t *testing.T) {
	e := limitedEngine(t, EngineRule{Name: "r", Condition: "value > 0"}, 10, time.Hour)
	snap := SnapshotEngine(e)
	snap.Seen["r"] = map[string]time.Time{identity.Key("odd"): time.Now()}
	if _, err := RestoreEngine(e, snap, time.Now()); err == nil {
		t.Fatal("invalid label pair accepted")
	}
}

func TestFailedSnapshotSavePreservesPreviousFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := StateSnapshot{Buffers: map[string]BufferSnapshot{"bad": {Entries: []EntrySnapshot{{Value: math.NaN()}}}}}
	if err := SaveSnapshot(path, bad); err == nil {
		t.Fatal("non-finite state saved")
	}
	if got, _ := os.ReadFile(path); string(got) != "previous" {
		t.Fatal("failed save destroyed old state")
	}
}
