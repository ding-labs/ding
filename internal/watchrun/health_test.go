package watchrun

import (
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func TestAcquisitionHealthDoesNotCallWaitingOrOverdueHealthy(t *testing.T) {
	now := time.Now()
	r := store.WatchRecord{Status: "running", Plan: plan.Compiled{Definition: watch.Definition{Spec: watch.Spec{Source: watch.Source{Every: "30s", Timeout: "5s"}}}}}
	if got, _ := acquisitionHealth(r, now); got != "waiting" {
		t.Fatal(got)
	}
	r.LastInputAt = now.Add(-70 * time.Second)
	if got, overdue := acquisitionHealth(r, now); got != "overdue" || overdue != 5 {
		t.Fatal(got, overdue)
	}
	r.LastError = "source_transport_failed"
	if got, _ := acquisitionHealth(r, now); got != "source-error" {
		t.Fatal(got)
	}
	r.Status = "paused"
	if got, _ := acquisitionHealth(r, now); got != "paused" {
		t.Fatal(got)
	}
	r.Status, r.LastError, r.LastInputAt = "running", "", now
	if got, _ := acquisitionHealth(r, now); got != "observed" {
		t.Fatal(got)
	}
}
