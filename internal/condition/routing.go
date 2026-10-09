package condition

import (
	"sort"

	"github.com/ding-labs/ding/internal/watch"
)

// SourceEntity holds source health before any grouped entity has been observed.
// It cannot collide with the canonical JSON identity of an actual entity.
const SourceEntity = "@source"

func (e *Evaluator) Route(existing []string, o watch.Observation) (keys []string, clearSource bool, err error) {
	if o.Health == "timer" && o.Entity != "" {
		return []string{o.Entity}, false, nil
	}
	if o.Health != "ok" {
		if len(existing) > 0 {
			sort.Strings(existing)
			return existing, false, nil
		}
		if len(e.definition.Spec.GroupBy) > 0 {
			return []string{SourceEntity}, false, nil
		}
	}
	if o.Health == "ok" {
		for field, want := range e.definition.Spec.Match {
			got, ok := o.Fields[field]
			known, matched := Compare(got, "eq", want)
			if !ok || !known || !matched {
				return nil, false, nil
			}
		}
		for _, key := range existing {
			if key == SourceEntity {
				clearSource = true
			}
		}
	}
	key, err := watch.EntityKey(e.definition.Spec.GroupBy, o.Fields)
	if err != nil {
		return nil, false, err
	}
	return []string{key}, clearSource, nil
}
func (e *Evaluator) SourceRecovered(o watch.Observation) watch.Event {
	id, _ := watch.Revision(struct {
		Watch, Revision, Entity, Type string
		Sequence                      int64
	}{e.definition.Metadata.ID, e.revision, SourceEntity, "source_recovered", o.Sequence})
	return watch.Event{ID: id, WatchID: e.definition.Metadata.ID, Revision: e.revision, Entity: SourceEntity, Type: "source_recovered", At: o.AcceptedAt, Message: "source_recovered", Evidence: []int64{o.Sequence}}
}
