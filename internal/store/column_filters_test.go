package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
)

func TestWatchColumnFiltersMatchDisplayedStates(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	c, _ := fixture(t)
	now := time.Now().UTC()
	if err := s.Update(ctx, func(tx *Tx) error {
		for _, id := range []string{"waiting", "open", "clear", "evaluated", "missing", "error", "old"} {
			definition := c.Definition
			definition.Metadata.ID = id
			if id == "evaluated" {
				definition.Spec.Policy.Trigger = "level"
				definition.Spec.Policy.Interval = "1m"
			}
			compiled, err := plan.Compile(definition)
			if err != nil {
				return err
			}
			r := WatchRecord{Plan: compiled, Status: "running", Generation: 1}
			if id != "waiting" {
				r.LastInputAt = now.Add(-30 * time.Minute)
			}
			if id == "old" {
				r.LastInputAt = now.Add(-8 * 24 * time.Hour)
			}
			if id == "error" {
				r.LastError = "source failed"
			}
			if err := tx.SaveWatch(r, now); err != nil {
				return err
			}
			if id != "waiting" {
				if err := tx.SaveEntity(id, "[]", compiled.Revision, map[string]any{"open": id == "open", "sourceUnhealthy": id == "error"}, now); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	missing := map[string][]string{"missing": {"TOKEN"}, "error": {"TOKEN"}}
	for _, tc := range []struct {
		name  string
		query ConsoleQuery
		want  []string
	}{
		{"waiting condition", ConsoleQuery{Condition: "waiting"}, []string{"waiting"}},
		{"open condition", ConsoleQuery{Condition: "open"}, []string{"open"}},
		{"clear condition", ConsoleQuery{Condition: "clear"}, []string{"clear", "error", "missing", "old"}},
		{"evaluated condition", ConsoleQuery{Condition: "evaluated"}, []string{"evaluated"}},
		{"source errors", ConsoleQuery{Health: "error"}, []string{"error"}},
		{"missing credentials", ConsoleQuery{Health: "missing"}, []string{"missing"}},
		{"waiting source", ConsoleQuery{Health: "waiting"}, []string{"waiting"}},
		{"received source", ConsoleQuery{Health: "received"}, []string{"clear", "evaluated", "old", "open"}},
		{"no input", ConsoleQuery{Input: "never"}, []string{"waiting"}},
		{"received input", ConsoleQuery{Input: "received"}, []string{"clear", "error", "evaluated", "missing", "old", "open"}},
		{"last hour", ConsoleQuery{Input: "hour"}, []string{"clear", "error", "evaluated", "missing", "open"}},
		{"last day", ConsoleQuery{Input: "day"}, []string{"clear", "error", "evaluated", "missing", "open"}},
		{"last week", ConsoleQuery{Input: "week"}, []string{"clear", "error", "evaluated", "missing", "open"}},
		{"combined", ConsoleQuery{Condition: "clear", Health: "received", Input: "day", Status: "running", Source: "http", Delivery: "clear"}, []string{"clear"}},
		{"no deliveries", ConsoleQuery{Delivery: "queued"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.query.Limit = 1
			if err := ValidateConsoleQuery(tc.query); err != nil {
				t.Fatal(err)
			}
			var got []string
			for {
				var page WatchPage
				if err := s.View(ctx, func(tx *Tx) error { var err error; page, err = tx.ConsoleWatches(tc.query, missing); return err }); err != nil {
					t.Fatal(err)
				}
				if page.Total != len(tc.want) || page.All != 7 {
					t.Fatalf("counts: %+v", page)
				}
				for _, w := range page.Watches {
					got = append(got, w.ID)
				}
				if !page.More {
					break
				}
				tc.query.Cursor = page.Cursor
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
	if err := s.View(ctx, func(tx *Tx) error {
		q := ConsoleQuery{Condition: "clear", Limit: 1}
		first, err := tx.ConsoleWatches(q, missing)
		if err != nil {
			return err
		}
		q.Cursor = first.Cursor
		q.Condition = "open"
		_, err = tx.ConsoleWatches(q, missing)
		if !errors.Is(err, ErrCursor) {
			t.Fatal("accepted cursor with changed condition", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []ConsoleQuery{{Condition: "typo"}, {Input: "typo"}, {Health: "typo"}, {Delivery: "typo"}} {
		if !errors.Is(ValidateConsoleQuery(q), ErrCursor) {
			t.Fatal("accepted invalid filter", q)
		}
	}
}

func TestWatchDeliveryColumnFiltersRespectDisplayPriority(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, d := fixture(t)
	if err := s.Update(ctx, func(tx *Tx) error {
		if err := transition(tx, c, d, "first", testNow); err != nil {
			return err
		}
		return transition(tx, c, d, "second", testNow)
	}); err != nil {
		t.Fatal(err)
	}
	check := func(filter string, want int) {
		t.Helper()
		if err := s.View(ctx, func(tx *Tx) error {
			page, err := tx.ConsoleWatches(ConsoleQuery{Limit: 10, Delivery: filter}, nil)
			if err == nil && page.Total != want {
				t.Fatalf("%s: got %d, want %d", filter, page.Total, want)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	check("queued", 1)
	check("failed", 0)
	check("clear", 0)
	if err := s.Update(ctx, func(tx *Tx) error {
		_, err := tx.sql.ExecContext(ctx, "UPDATE outbox SET status='permanent' WHERE event_id='event-first'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	check("queued", 0)
	check("failed", 1)
	check("pending", 1)
	if err := s.Update(ctx, func(tx *Tx) error {
		_, err := tx.sql.ExecContext(ctx, "UPDATE outbox SET status='delivered'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	check("queued", 0)
	check("failed", 0)
	check("clear", 1)
}
