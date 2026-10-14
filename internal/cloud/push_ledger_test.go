package cloud

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

type gatedLedger struct {
	bytes.Buffer
	entered chan struct{}
	release chan struct{}
	err     error
}

func (g *gatedLedger) Sync() error { g.entered <- struct{}{}; <-g.release; return g.err }

func TestGroupedLedgerWaitsForSyncAndPropagatesFailure(t *testing.T) {
	failure := errors.New("sync failed")
	g := &gatedLedger{entered: make(chan struct{}, 1), release: make(chan struct{}), err: failure}
	record, closeLedger := groupedPushLedger(g, 8)
	done := make(chan error, 1)
	go func() { done <- record(pushLedgerEntry{Stage: "offered", Key: "one"}) }()
	<-g.entered
	select {
	case <-done:
		t.Fatal("returned before sync")
	default:
	}
	close(g.release)
	if err := <-done; !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := record(pushLedgerEntry{Stage: "offered", Key: "two"}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	closeLedger()
	if bytes.Contains(g.Bytes(), []byte("two")) {
		t.Fatal("wrote after permanent failure")
	}
}

func TestGroupedLedgerFlushesPartialBatch(t *testing.T) {
	g := &gatedLedger{entered: make(chan struct{}, 1), release: make(chan struct{})}
	close(g.release)
	record, closeLedger := groupedPushLedger(g, 32)
	done := make(chan error, 1)
	go func() { done <- record(pushLedgerEntry{Stage: "offered", Key: "one"}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("partial batch stuck")
	}
	closeLedger()
	if !bytes.Contains(g.Bytes(), []byte("one")) {
		t.Fatal("entry missing")
	}
}
