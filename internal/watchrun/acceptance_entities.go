package watchrun

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

// This cache belongs to one acceptance transaction. Healthy observations load
// touched entities; source-health inputs retain the evaluator's full fanout.
type acceptanceEntities struct {
	tx       *store.Tx
	id       string
	revision string
	now      time.Time
	states   map[string]json.RawMessage
	pending  map[string]json.RawMessage
	known    map[string]bool
	all      bool
	n        int
}

func newAcceptanceEntities(tx *store.Tx, id, revision string, now time.Time) *acceptanceEntities {
	return &acceptanceEntities{tx: tx, id: id, revision: revision, now: now,
		states: map[string]json.RawMessage{}, pending: map[string]json.RawMessage{}, known: map[string]bool{}, n: -1}
}

func (s *acceptanceEntities) flush() error {
	if len(s.pending) == 0 {
		return nil
	}
	if err := s.tx.SaveEntities(s.id, s.revision, s.pending, s.now); err != nil {
		return err
	}
	clear(s.pending)
	return nil
}

func (s *acceptanceEntities) preload(e *condition.Evaluator, observations []watch.Observation) error {
	if len(observations) < 2 {
		return nil
	}
	wanted := map[string]bool{condition.SourceEntity: true}
	for _, o := range observations {
		if o.Health != "ok" {
			continue // Source-health fanout is loaded when evaluated in order.
		}
		keys, _, err := e.Route(nil, o)
		if err != nil {
			return err
		}
		for _, key := range keys {
			wanted[key] = true
		}
	}
	keys := make([]string, 0, len(wanted))
	for key := range wanted {
		keys = append(keys, key)
	}
	states, err := s.tx.EntitySubset(s.id, keys)
	if err != nil {
		return err
	}
	s.states, s.known = states, wanted
	return nil
}

func (s *acceptanceEntities) load(key string) (json.RawMessage, bool, error) {
	if !s.all && !s.known[key] {
		data, err := s.tx.Entity(s.id, key)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, false, err
		}
		s.known[key] = true
		if err == nil {
			s.states[key] = data
		}
	}
	data, ok := s.states[key]
	return data, ok, nil
}

func (s *acceptanceEntities) route(e *condition.Evaluator, o watch.Observation) ([]string, bool, error) {
	var existing []string
	if o.Health == "ok" {
		_, exists, err := s.load(condition.SourceEntity)
		if err != nil {
			return nil, false, err
		}
		if exists {
			existing = []string{condition.SourceEntity}
		}
	} else if o.Health != "timer" || o.Entity == "" {
		if !s.all {
			// A source-health input must fan out over earlier writes in this
			// same batch, including newly created groups.
			if err := s.flush(); err != nil {
				return nil, false, err
			}
			states, err := s.tx.Entities(s.id)
			if err != nil {
				return nil, false, err
			}
			s.states, s.all, s.n = states, true, len(states)
		}
		for key := range s.states {
			existing = append(existing, key)
		}
	}
	return e.Route(existing, o)
}

func (s *acceptanceEntities) count() (int, error) {
	if s.n < 0 {
		n, err := s.tx.EntityCount(s.id)
		if err != nil {
			return 0, err
		}
		s.n = n
	}
	return s.n, nil
}

func (s *acceptanceEntities) saved(key string, data json.RawMessage) {
	if _, exists := s.states[key]; !exists && s.n >= 0 {
		s.n++
	}
	s.states[key], s.known[key] = data, true
	s.pending[key] = data
}

func (s *acceptanceEntities) removed(key string) {
	if _, exists := s.states[key]; exists && s.n >= 0 {
		s.n--
	}
	delete(s.states, key)
	delete(s.pending, key)
	s.known[key] = true
}
