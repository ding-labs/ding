package condition

import (
	"bytes"
	"encoding/json"
	"time"
	"unicode/utf8"
)

func (e *Evaluator) signal(state *State, value any, sequence int64, now time.Time) (bool, bool, string) {
	if e.definition.Spec.Condition.Operator == "changed" {
		if number, ok := Number(value); ok {
			value = number
		} else {
			switch v := value.(type) {
			case nil, bool:
			case string:
				if !utf8.ValidString(v) {
					return false, false, "wrong_field_type"
				}
			default:
				return false, false, "wrong_field_type"
			}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return false, false, "wrong_field_type"
		}
		changed := len(state.Baseline) > 0 && !bytes.Equal(state.Baseline, encoded)
		state.Evidence = nil
		if changed && state.BaselineSequence > 0 {
			state.Evidence = []int64{state.BaselineSequence}
		}
		state.Baseline = encoded
		state.BaselineSequence = sequence
		return true, changed, ""
	}
	id, ok := value.(string)
	if !ok || id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return false, false, "event_id_must_be_a_nonempty_string_up_to_256_bytes"
	}
	if state.Seen == nil {
		state.Seen = map[string]time.Time{}
	}
	for id, expiry := range state.Seen {
		if !now.Before(expiry) {
			delete(state.Seen, id)
		}
	}
	if expiry, exists := state.Seen[id]; exists && now.Before(expiry) {
		return true, false, ""
	}
	if len(state.Seen) >= e.definition.Spec.Limits.MaxSamples {
		return false, false, "dedup_budget"
	}
	duration, _ := time.ParseDuration(e.definition.Spec.Condition.DedupFor)
	state.Seen[id] = now.Add(duration)
	return true, true, ""
}
