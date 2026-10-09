package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/ding-labs/ding/internal/watch"
	"testing"
)

func TestConsoleEventSnapshotPagingFiltersAndRetention(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	seed(t, s)
	p, _ := fixture(t)
	add := func(id string) {
		t.Helper()
		if err := s.Update(ctx, func(tx *Tx) error {
			_, err := tx.AppendEvent(watch.Event{ID: id, WatchID: "health", Revision: p.Revision, At: testNow, Type: "firing", Message: "Test"})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 4; i++ {
		add(fmt.Sprint(i))
	}
	q := ConsoleQuery{Watch: "health", Limit: 2}
	var first EventSummaryPage
	err := s.View(ctx, func(tx *Tx) error { var err error; first, err = tx.ConsoleEvents(q); return err })
	if err != nil || len(first.Events) != 2 || first.Events[0].ID != "4" || first.Total != 4 || !first.More {
		t.Fatal(first, err)
	}
	add("5")
	q.Cursor = first.Cursor
	err = s.View(ctx, func(tx *Tx) error {
		page, err := tx.ConsoleEvents(q)
		if err != nil {
			return err
		}
		if len(page.Events) != 2 || page.Events[0].ID != "2" || page.More || page.Total != 4 {
			t.Fatal(page)
		}
		live, err := tx.EventPage("health", first.FollowCursor, 10)
		if err != nil {
			return err
		}
		if len(live.Events) != 1 || live.Events[0].ID != "5" {
			t.Fatal(live)
		}
		wrong := q
		wrong.Type = "recovery"
		if _, err = tx.ConsoleEvents(wrong); !errors.Is(err, ErrCursor) {
			t.Fatal("filter cursor accepted", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Update(ctx, func(tx *Tx) error { _, err := tx.sql.ExecContext(ctx, "DELETE FROM events WHERE id='1'"); return err }); err != nil {
		t.Fatal(err)
	}
	if err = s.View(ctx, func(tx *Tx) error { _, err := tx.ConsoleEvents(q); return err }); !errors.Is(err, ErrCursorExpired) {
		t.Fatal("retention gap hidden", err)
	}
}
func TestConsoleWatchCompleteCountsAndAttention(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	ctx := context.Background()
	err := s.View(ctx, func(tx *Tx) error {
		page, err := tx.ConsoleWatches(ConsoleQuery{Limit: 1, Attention: "yes"}, map[string][]string{"health": {"API_KEY"}})
		if err != nil {
			return err
		}
		if page.Total != 1 || page.All != 1 || page.Attention != 1 || len(page.Watches) != 1 || page.Watches[0].Missing[0] != "API_KEY" {
			t.Fatal(page)
		}
		page, err = tx.ConsoleWatches(ConsoleQuery{Limit: 1, Search: "no match"}, nil)
		if err != nil {
			return err
		}
		if page.Total != 0 || page.All != 1 || len(page.Watches) != 0 {
			t.Fatal(page)
		}
		if _, err = tx.ConsoleDeliveries(ConsoleQuery{Limit: 1}); err != nil {
			return err
		}
		if _, err = tx.ConsoleDestinations(ConsoleQuery{Limit: 1}); err != nil {
			return err
		}
		_, err = tx.SecretNames()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestConsoleRejectsUnboundedOrInvalidQueries(t *testing.T) {
	s := openTest(t, t.TempDir())
	for _, q := range []ConsoleQuery{{Limit: 0}, {Limit: 101}, {Limit: 10, Cursor: "nonsense"}, {Limit: 10, From: "yesterday"}} {
		err := s.View(context.Background(), func(tx *Tx) error { _, err := tx.ConsoleEvents(q); return err })
		if !errors.Is(err, ErrCursor) {
			t.Fatal(q, err)
		}
	}
}
