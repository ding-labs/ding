package store

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/watch"
)

func TestPostgresIndependentFacadesSerializeWorkspaceWrites(t *testing.T) {
	s := openPostgresTestStore(t)
	s.db.SetMaxOpenConns(4)
	other, err := OpenPostgres(context.Background(), s.db, s.workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	seed(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	failures := make(chan error, 100)
	stores := []*Store{s, other}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			failures <- stores[i%2].Update(ctx, func(tx *Tx) error {
				r, err := tx.Watch("health")
				if err != nil {
					return err
				}
				n, _ := strconv.Atoi(r.Cursor)
				if _, err = tx.AppendObservation("health", r.Plan.Revision, 1, watch.Observation{InputID: fmt.Sprint(i), AcceptedAt: testNow}); err != nil {
					return err
				}
				return tx.Checkpoint("health", 1, testNow, testNow, strconv.Itoa(n+1))
			})
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = s.View(ctx, func(tx *Tx) error {
		r, err := tx.Watch("health")
		if err != nil {
			return err
		}
		if r.Cursor != "100" {
			t.Fatal("concurrent update lost state", r.Cursor)
		}
		usage, err := tx.Usage()
		if err != nil {
			return err
		}
		if usage.Observations != 100 {
			t.Fatal("concurrent rows lost", usage)
		}
		var high int64
		if err = tx.sql.QueryRow("SELECT observation_sequence FROM workspaces").Scan(&high); err != nil || high != 100 {
			t.Fatal("concurrent ranges have gaps", high, err)
		}
		checkPostgresLogicalBytes(t, tx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresSnapshotOverlapsCommittedWrite(t *testing.T) {
	s := openPostgresTestStore(t)
	s.db.SetMaxOpenConns(2)
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
				t.Error("read snapshot changed after commit")
			}
			return err
		})
	}()
	<-read
	if err := s.Update(ctx, func(tx *Tx) error { return tx.Checkpoint("health", 1, testNow, testNow, "committed") }); err != nil {
		t.Fatal(err)
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
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
