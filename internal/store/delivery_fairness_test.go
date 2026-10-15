package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/pgtest"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

func TestDeliveryIndependentStreamsHaveFairTurns(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			var s *Store
			if backend == "sqlite" {
				s = openTest(t, t.TempDir())
			} else {
				f := pgtest.New(t)
				if err := MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
					t.Fatal(err)
				}
				var err error
				s, err = OpenPostgres(ctx, f.App, strings.Repeat("a", 64))
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
			}
			seed(t, s)
			c, d := fixture(t)
			if err := s.Update(ctx, func(tx *Tx) error {
				definition := c.Definition
				definition.Metadata.ID = "other"
				other, err := plan.Compile(definition)
				if err != nil {
					return err
				}
				if err := tx.SaveWatch(WatchRecord{Plan: other, Generation: 1, Status: "running"}, testNow); err != nil {
					return err
				}
				for _, stream := range []string{"health", "other"} {
					n := 500
					revision := c.Revision
					if stream == "other" {
						revision = other.Revision
						n = 2
					}
					for i := range n {
						id := fmt.Sprintf("%s-%d", stream, i)
						if _, err := tx.AppendEvent(watch.Event{ID: id, WatchID: stream, Revision: revision, Type: "firing", At: testNow}); err != nil {
							return err
						}
						if _, err := tx.Enqueue(Intent{EventID: id, WatchID: stream, DestinationID: "ops", DestinationRevision: d.Revision, Payload: []byte(`{}`), NextAt: testNow, CreatedAt: testNow}); err != nil {
							return err
						}
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			first, err := s.Claim(ctx, testNow, time.Minute)
			if err != nil || first == nil || first.WatchID != "health" {
				t.Fatal("first stream", err)
			}
			second, err := s.Claim(ctx, testNow, time.Minute)
			if err != nil || second == nil || second.WatchID != "other" {
				t.Fatal("independent stream blocked by active lease", err)
			}
			if more, err := s.Claim(ctx, testNow, time.Minute); err != nil || more != nil {
				t.Fatal("overtook a stream lease", err)
			}
			for _, intent := range []*Intent{first, second} {
				if err = s.Finish(ctx, *intent, delivery.Result{Outcome: delivery.Delivered}, testNow, testNow); err != nil {
					t.Fatal(err)
				}
			}
			for _, want := range []string{"health", "other", "health"} {
				next, err := s.Claim(ctx, testNow, time.Minute)
				if err != nil || next == nil || next.WatchID != want {
					t.Fatal("backlog starved stream", want, err)
				}
				if err = s.Finish(ctx, *next, delivery.Result{Outcome: delivery.Delivered}, testNow, testNow); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
