package watchrun

import (
	"encoding/json"
	"errors"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

// This cache belongs to one acceptance transaction. Healthy observations load
// touched entities; source-health inputs retain the evaluator's full fanout.
type acceptanceEntities struct {
	tx     *store.Tx
	id     string
	states map[string]json.RawMessage
	known  map[string]bool
	all    bool
	n      int
}

func newAcceptanceEntities(tx *store.Tx, id string) *acceptanceEntities {
	return &acceptanceEntities{tx: tx, id: id, states: map[string]json.RawMessage{}, known: map[string]bool{}, n: -1}
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
}

func (s *acceptanceEntities) removed(key string) {
	if _, exists := s.states[key]; exists && s.n >= 0 {
		s.n--
	}
	delete(s.states, key)
	s.known[key] = true
}
