package watchrun

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func TestAcceptanceEntitiesUsesTargetedReadsAndPreservesFanout(t *testing.T) {
	a, _ := setup(t)
	m := strings.Replace(manifest("https://example.com"), "  condition:", "  groupBy: [host]\n  condition:", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	r, err := a.Record(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	e, err := condition.New(r.Plan.Definition, r.Plan.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Update(ctx, func(tx *store.Tx) error {
		for _, key := range []string{"a", "b", condition.SourceEntity} {
			if err := tx.SaveEntity("api", key, r.Plan.Revision, condition.State{Entity: key}, start); err != nil {
				return err
			}
		}
		s := newAcceptanceEntities(tx, "api")
		keys, clear, err := s.route(e, watch.Observation{Health: "ok", Fields: map[string]any{"host": "new"}})
		if err != nil || len(keys) != 1 || !clear || s.all || len(s.states) != 1 {
			t.Fatal(keys, clear, s.all, len(s.states), err)
		}
		if _, exists, err := s.load(keys[0]); err != nil || exists {
			t.Fatal(exists, err)
		}
		if n, err := s.count(); err != nil || n != 3 {
			t.Fatal(n, err)
		}
		if err := tx.DeleteEntity("api", condition.SourceEntity); err != nil {
			return err
		}
		s.removed(condition.SourceEntity)
		if err := tx.SaveEntity("api", keys[0], r.Plan.Revision, condition.State{Entity: keys[0]}, start); err != nil {
			return err
		}
		s.saved(keys[0], json.RawMessage(`{}`))
		if n, err := s.count(); err != nil || n != 3 {
			t.Fatal(n, err)
		}
		fanout, _, err := s.route(e, watch.Observation{Health: "error"})
		want, _, _ := e.Route([]string{"a", "b", keys[0]}, watch.Observation{Health: "error"})
		if err != nil || !s.all || !reflect.DeepEqual(fanout, want) {
			t.Fatal(fanout, want, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTargetedAcceptanceKeepsBatchOrderQuotaAndRollback(t *testing.T) {
	a, _ := setup(t)
	m := strings.Replace(manifest("https://example.com"), "  condition:", "  groupBy: [host]\n  limits: {maxEntities: 1}\n  condition:", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	r, err := a.Record(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	observation := func(host string) watch.Observation {
		return watch.Observation{Health: "ok", Fields: map[string]any{"host": host, "http.status": 500}}
	}
	batch := source.Batch{Observations: []watch.Observation{observation("a"), observation("a")}}
	first, err := a.Accept(ctx, r, batch, "same-group", start.Add(time.Second))
	if err != nil || first.Last-first.First+1 != 2 {
		t.Fatal(first, err)
	}
	before, err := a.Inspect(ctx, "api")
	if err != nil || len(before.Entities) != 1 {
		t.Fatal(before, err)
	}
	for _, raw := range before.Entities {
		var state condition.State
		if err := json.Unmarshal(raw, &state); err != nil || state.Matches != 2 || state.Open {
			t.Fatal(state, err)
		}
	}
	// The first observation would fire; admitting the second group must roll
	// that transition, its delivery and both observation rows back together.
	batch.Observations[1] = observation("b")
	if _, err := a.Accept(ctx, r, batch, "over-limit", start.Add(2*time.Second)); !errors.Is(err, ErrQuota) {
		t.Fatal("new group did not respect quota", err)
	}
	after, err := a.Inspect(ctx, "api")
	if err != nil || !reflect.DeepEqual(before.Entities, after.Entities) || len(after.Deliveries) != 0 {
		t.Fatal(after, err)
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error {
		u, err := tx.Usage()
		if err == nil && u.Observations != 2 {
			t.Fatal("partial rejected batch remained", u)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
