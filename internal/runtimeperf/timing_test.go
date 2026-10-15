package runtimeperf

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFiniteConcurrentMeasurements(t *testing.T) {
	before := timing[SQLite][WriterWait].count.Load()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				Observe(SQLite, WriterWait, 5*time.Millisecond)
			}
		})
	}
	wg.Wait()
	Observe(Backend(255), Operation(255), time.Second)
	if timing[SQLite][WriterWait].count.Load()-before != 800 {
		t.Fatal("lost concurrent measurements")
	}
	var output bytes.Buffer
	Write(&output)
	if !strings.Contains(output.String(), `operation="writer_wait",le="0.01"`) || strings.Contains(output.String(), "255") {
		t.Fatal("invalid labels")
	}
}
