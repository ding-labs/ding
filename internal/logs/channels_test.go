package logs

import (
	"testing"
	"time"
)

func TestChannelCountsDoNotImplySeverityAndDropsInvalidate(t *testing.T) {
	at := time.Unix(100, 0)
	w, _ := NewWindow(WindowConfig{Window: time.Second, MinEvents: 1}, at)
	w.AddFrom(at, Classification{Level: Info}, "stderr")
	w.AddFrom(at, Classification{Level: Warning}, "stdout")
	w.AddFrom(at, Classification{}, "pipe")
	s := w.Snapshot(at.Add(time.Second))
	if s.Total != 3 || s.Stdout != 1 || s.Stderr != 1 || s.Other != 1 || s.Errors != 0 || s.Classified != 2 {
		t.Fatal(s)
	}
	w.Drop(at)
	s = w.Snapshot(at.Add(time.Second))
	if s.Dropped != 1 || s.Ready || s.Total != 3 {
		t.Fatal(s)
	}
}
