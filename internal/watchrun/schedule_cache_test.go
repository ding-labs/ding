package watchrun

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/runtimeperf"
)

func snapshotCount() int64 {
	var metrics bytes.Buffer
	runtimeperf.Write(&metrics)
	var total int64
	for _, line := range strings.Split(metrics.String(), "\n") {
		if strings.HasPrefix(line, "ding_runtime_operation_seconds_count{") && strings.Contains(line, `operation="snapshot"}`) {
			var n int64
			_, _ = fmt.Sscan(line[strings.LastIndexByte(line, ' ')+1:], &n)
			total += n
		}
	}
	return total
}

func TestMaintenanceSnapshotServesConcurrentIdleWorkers(t *testing.T) {
	a, _ := setup(t)
	var schedule scheduleCache
	if err := a.maintain(ctx, start, &schedule); err != nil {
		t.Fatal(err)
	}
	before := snapshotCount()
	var workers sync.WaitGroup
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			var delivery deliveryCache
			if ready, err := delivery.ready(ctx, a.Store, start.Add(time.Second), &schedule); err != nil || ready {
				t.Error("empty maintenance snapshot not reusable", ready, err)
			}
			if _, err := schedule.load(ctx, a.Store, start.Add(time.Second)); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	if snapshotCount() != before {
		t.Fatal("idle workers reread maintenance snapshot")
	}
}

func TestWorkerCachesRefreshAfterCommit(t *testing.T) {
	a, _ := setup(t)
	var schedule scheduleCache
	var delivery deliveryCache
	if _, err := schedule.load(ctx, a.Store, start); err != nil {
		t.Fatal(err)
	}
	if ready, err := delivery.ready(ctx, a.Store, start, &schedule); err != nil || ready {
		t.Fatal(ready, err)
	}
	before := snapshotCount()
	for range 100 {
		if _, err := schedule.load(ctx, a.Store, start.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if ready, err := delivery.ready(ctx, a.Store, start.Add(time.Second), &schedule); err != nil || ready {
			t.Fatal(ready, err)
		}
	}
	if snapshotCount() != before {
		t.Fatal("idle worker checks reopened SQL snapshots")
	}
	r := apply(t, a, "https://example.com")
	state, err := schedule.load(ctx, a.Store, start.Add(time.Second))
	if err != nil || len(state.Watches) != 1 {
		t.Fatal("new watch hidden by cache", err)
	}
	for i := 1; i <= 3; i++ {
		input(t, a, r, i, 500)
	}
	if ready, err := delivery.ready(ctx, a.Store, start.Add(20*time.Second), &schedule); err != nil || !ready {
		t.Fatal("new delivery hidden by cache", ready, err)
	}
}

func TestWorkerCachePeriodicallyReconstructsDurableState(t *testing.T) {
	a, _ := setup(t)
	var schedule scheduleCache
	if _, err := schedule.load(ctx, a.Store, start); err != nil {
		t.Fatal(err)
	}
	before := snapshotCount()
	if _, err := schedule.load(ctx, a.Store, start.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if snapshotCount() != before+1 {
		t.Fatal("missed periodic durable reconciliation")
	}
	before = snapshotCount()
	if _, err := schedule.load(ctx, a.Store, start.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if snapshotCount() != before+1 {
		t.Fatal("clock rollback retained a future cache")
	}
}
