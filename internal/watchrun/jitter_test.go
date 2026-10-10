package watchrun

import (
	"testing"
	"time"
)

func TestHostedJitterNeverAcceleratesAndLeavesLocalCadenceAlone(t *testing.T) {
	a := &App{}
	now := time.Now()
	if got := a.nextPoll(now, 5*time.Minute, "a"); !got.Equal(now.Add(5 * time.Minute)) {
		t.Fatal(got)
	}
	a.SchedulingJitter = 5 * time.Second
	aTime, bTime := a.nextPoll(now, 5*time.Minute, "a"), a.nextPoll(now, 5*time.Minute, "b")
	if aTime.Equal(bTime) {
		t.Fatal("watches not spread")
	}
	for _, v := range []time.Time{aTime, bTime} {
		if v.Before(now.Add(5*time.Minute)) || !v.Before(now.Add(5*time.Minute+5*time.Second)) {
			t.Fatal("jitter exceeded bounds")
		}
	}
}
