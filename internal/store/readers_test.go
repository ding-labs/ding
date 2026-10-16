package store

import (
	"context"
	"testing"
	"time"
)

func TestReadSnapshotOverlapsWriterAndRemainsConsistent(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	read, committed := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.View(ctx, func(tx *Tx) error {
			before, err := tx.Watch("health")
			if err != nil {
				return err
			}
			close(read)
			select {
			case <-committed:
			case <-ctx.Done():
				return ctx.Err()
			}
			after, err := tx.Watch("health")
			if err == nil && before.Cursor != after.Cursor {
				t.Error("snapshot changed across committed writer")
			}
			return err
		})
	}()
	<-read
	if err := s.Update(ctx, func(tx *Tx) error {
		return tx.Checkpoint("health", 1, testNow, testNow, "committed")
	}); err != nil {
		t.Fatal("writer blocked by snapshot", err)
	}
	close(committed)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := s.View(ctx, func(tx *Tx) error {
		r, err := tx.Watch("health")
		if err == nil && r.Cursor != "committed" {
			t.Error("new snapshot missed commit")
		}
		if _, err := tx.sql.Exec("DELETE FROM watches"); err == nil {
			t.Error("snapshot connection allowed mutation")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
