package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitingWriterCancelsWithoutEnteringTransaction(t *testing.T) {
	s := openTest(t, t.TempDir())
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.Update(context.Background(), func(*Tx) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := s.Update(ctx, func(*Tx) error {
		t.Error("cancelled writer entered transaction")
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
