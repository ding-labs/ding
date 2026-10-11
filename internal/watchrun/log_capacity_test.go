package watchrun

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDaemonReaderCapacityIsVisibleAndDoesNotRepeat(t *testing.T) {
	a, _ := setup(t)
	w := logWorkers{app: a, active: map[string]logWorker{}, retry: map[string]time.Time{}}
	for n := range 9 {
		id := fmt.Sprintf("reader-%d", n)
		manifest := strings.Replace(nativeLogManifest, "id: output", "id: "+id, 1)
		manifest = strings.Replace(manifest, "window: 1s", "input: {type: file, path: /tmp/log-fixture}, window: 1s", 1)
		if _, err := a.Apply(ctx, ApplyRequest{Manifest: manifest}); err != nil {
			t.Fatal(err)
		}
		if n < 8 {
			w.active[id] = logWorker{generation: 1, cancel: func() {}, done: make(chan struct{})}
		}
	}
	records, _ := a.List(ctx)
	w.reconcile(ctx, records, start)
	r, _ := a.Record(ctx, "reader-8")
	if !strings.Contains(r.Cursor, `"status":"capacity"`) {
		t.Fatal(r.Cursor)
	}
	i, _ := a.Inspect(ctx, "reader-8")
	if len(i.Entities) == 0 {
		t.Fatal("no capacity evidence")
	}
	records, _ = a.List(ctx)
	w.reconcile(ctx, records, start.Add(time.Second))
	after, _ := a.Record(ctx, "reader-8")
	if !after.LastInputAt.Equal(r.LastInputAt) {
		t.Fatal("repeated capacity observation")
	}
}
