package cloud

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestPlannedJournalDecouplesSyncAndDrains(t *testing.T) {
	g := &gatedLedger{entered: make(chan struct{}, 8), release: make(chan struct{})}
	record, finish := plannedPushJournal(g, 2)
	if err := record(pushLedgerEntry{Key: "one"}); err != nil {
		t.Fatal(err)
	}
	<-g.entered
	// Sync is blocked, but another response can be recorded without waiting.
	if err := record(pushLedgerEntry{Key: "two"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- finish() }()
	select {
	case <-done:
		t.Fatal("finished before durable flush")
	default:
	}
	close(g.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("drain stuck")
	}
	if !bytes.Contains(g.Bytes(), []byte("two")) {
		t.Fatal("lost queued response")
	}
}

func TestPlannedJournalReportsFinalSyncFailure(t *testing.T) {
	failure := errors.New("disk failed")
	g := &gatedLedger{entered: make(chan struct{}, 8), release: make(chan struct{}), err: failure}
	close(g.release)
	record, finish := plannedPushJournal(g, 2)
	_ = record(pushLedgerEntry{Key: "one"})
	if err := finish(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
