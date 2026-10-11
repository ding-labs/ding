package logs

import (
	"testing"
	"time"
)

func TestCollectorDrainAndBoundedAnalysis(t *testing.T) {
	c, err := NewCollector(ParserConfig{}, WindowConfig{Window: 10 * time.Second, MinEvents: 1})
	if err != nil {
		t.Fatal(err)
	}
	a, b := c.Stream(), c.Stream()
	a.Write([]byte("WARN a\n"))
	b.Write([]byte("ERROR b"))
	a.Close()
	b.Close()
	c.Snapshot() // Barrier before reading the underlying deterministic window.
	c.Close()
	s := c.window.Snapshot(time.Now().Add(time.Second))
	if s.Total != 2 || s.Warnings != 1 || s.Errors != 1 {
		t.Fatal(s)
	}
	if len(c.queue) != 0 {
		t.Fatal("close must drain queued records")
	}
}
