package watchrun

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func TestConcurrentPushUsesTransactionAcceptanceTime(t *testing.T) {
	a, _ := setup(t)
	_, err := a.Apply(ctx, ApplyRequest{Manifest: `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: concurrent}
spec:
  source: {type: push, fields: {value: value}}
  condition: {field: value, operator: gte, value: 1}
  policy: {trigger: level, interval: 0s}
`})
	if err != nil {
		t.Fatal(err)
	}
	r, err := a.Record(ctx, "concurrent")
	if err != nil {
		t.Fatal(err)
	}
	var clockCalls atomic.Int64
	a.Now = func() time.Time {
		now := time.Now().UTC()
		if clockCalls.Add(1)%2 == 1 {
			time.Sleep(time.Millisecond)
		}
		return now
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := a.IngestReserved(ctx, r, []byte(`{"value":2}`), fmt.Sprint(i)); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	var events []watch.Event
	err = a.Store.View(ctx, func(tx *store.Tx) error { var e error; events, e = tx.Events("concurrent", 0, 1000); return e })
	if err != nil {
		t.Fatal(err)
	}
	firings := 0
	for _, event := range events {
		if event.Type == "source_error" {
			t.Fatal("ordinary contention became a clock discontinuity", event)
		}
		if event.Type == "firing" {
			firings++
		}
	}
	if firings != 100 {
		t.Fatal("accepted inputs lost firing", firings)
	}
}
