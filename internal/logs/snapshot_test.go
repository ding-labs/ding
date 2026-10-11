package logs

import (
	"testing"
	"time"
)

func TestQueuedSnapshotKeepsItsBoundaryWhenClassificationIsLate(t *testing.T) {
	c, _ := NewCollector(ParserConfig{}, WindowConfig{Window: time.Second, MinEvents: 1})
	defer c.Close()
	at := time.Unix(100, 0)
	c.window.Reset(at)
	data := []byte("WARN complete")
	c.mu.Lock()
	c.queuedBytes += len(data)
	c.queue <- record{data: data, at: at.Add(800 * time.Millisecond)}
	c.mu.Unlock()
	result := make(chan Summary, 1)
	// The barrier was requested before second 101, even if classification and
	// delivery occur later. Its completed window cannot include second 100 yet.
	c.queue <- record{snapshot: result, at: at.Add(900 * time.Millisecond)}
	if s := <-result; !s.WindowEnd.Equal(at) || s.Total != 0 {
		t.Fatal(s)
	}
	c.queue <- record{snapshot: result, at: at.Add(time.Second)}
	if s := <-result; s.Total != 1 || s.Warnings != 1 {
		t.Fatal(s)
	}
}
