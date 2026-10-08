package condition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"text/template"
	"time"

	"github.com/ding-labs/ding/internal/watch"
)

type Sample struct {
	At       time.Time `json:"at"`
	Value    float64   `json:"value"`
	Sequence int64     `json:"sequence"`
}
type State struct {
	IncidentEvent   string    `json:"incidentEvent,omitempty"`
	HealthEvent     string    `json:"healthEvent,omitempty"`
	FreshSequence   int64     `json:"freshSequence,omitempty"`
	MissingAt       time.Time `json:"missingAt,omitempty"`
	Entity          string    `json:"entity"`
	Open            bool      `json:"open"`
	Matches         int       `json:"matches"`
	Recoveries      int       `json:"recoveries"`
	LastAt          time.Time `json:"lastAt"`
	LastFired       time.Time `json:"lastFired"`
	LastSequence    int64     `json:"lastSequence"`
	SourceUnhealthy bool      `json:"sourceUnhealthy"`
	Samples         []Sample  `json:"samples,omitempty"`
	OverflowUntil   time.Time `json:"overflowUntil,omitempty"`
	Evidence        []int64   `json:"evidence,omitempty"`
	// Baseline and Seen are populated by change/new-event adapters in P11.
	Baseline json.RawMessage      `json:"baseline,omitempty"`
	Seen     map[string]time.Time `json:"seen,omitempty"`
}
type Transition struct {
	State   State         `json:"state"`
	Events  []watch.Event `json:"events"`
	Known   bool          `json:"known"`
	Matched bool          `json:"matched"`
	Reason  string        `json:"reason,omitempty"`
}
type Evaluator struct {
	definition       watch.Definition
	revision         string
	expression       ConditionExpr
	maxWindow        time.Duration
	interval, maxGap time.Duration
	message          *template.Template
}

func New(def watch.Definition, revision string) (*Evaluator, error) {
	// Call with a normalized compiled definition. Copy owned data so evaluation
	// never mutates a manifest or earlier transaction state.
	data, err := json.Marshal(def)
	if err != nil {
		return nil, err
	}
	var copied watch.Definition
	if err = json.Unmarshal(data, &copied); err != nil {
		return nil, err
	}
	e := &Evaluator{definition: copied, revision: revision}
	if def.Spec.Condition.Operator == "changed" || def.Spec.Condition.Operator == "new-event" {
		return nil, fmt.Errorf("condition capability is not implemented yet")
	}
	if def.Spec.Policy.Consecutive < 1 || def.Spec.Policy.RecoverAfter < 1 || def.Spec.Limits.MaxSamples < 1 {
		return nil, fmt.Errorf("definition must be compiled before evaluation")
	}
	if def.Spec.Condition.Numeric != "" {
		e.expression, err = ParseExpression(def.Spec.Condition.Numeric)
		if err != nil {
			return nil, err
		}
		for _, w := range e.expression.Windows() {
			if w.RunBounded {
				return nil, fmt.Errorf("run windows unsupported")
			}
			if w.Window > e.maxWindow {
				e.maxWindow = w.Window
			}
		}
	}
	e.interval, _ = time.ParseDuration(def.Spec.Policy.Interval)
	e.maxGap, _ = time.ParseDuration(def.Spec.Policy.MaxGap)
	e.message, err = template.New("message").Option("missingkey=error").Parse(def.Spec.Message)
	return e, err
}

// Evaluate is deterministic and has no ambient clock, environment, or I/O. The
// returned state becomes authoritative only after the caller's durable commit.
func (e *Evaluator) Evaluate(previous State, o watch.Observation, now time.Time) (Transition, error) {
	state := previous
	state.Samples = append([]Sample(nil), previous.Samples...)
	state.Evidence = append([]int64(nil), previous.Evidence...)
	result := Transition{State: state, Events: []watch.Event{}}
	if now.IsZero() || o.Sequence < 1 || !now.Equal(o.AcceptedAt) {
		return result, fmt.Errorf("accepted time and positive sequence are required")
	}
	if o.Sequence <= state.LastSequence {
		return result, fmt.Errorf("observation sequence must increase")
	}
	key, err := watch.EntityKey(e.definition.Spec.GroupBy, o.Fields)
	if err != nil {
		return result, err
	}
	if o.Health == "timer" && o.Entity != "" {
		key = o.Entity
	}
	if state.Entity != "" {
		if o.Health == "ok" && state.Entity != key {
			return result, fmt.Errorf("observation belongs to another entity")
		}
		key = state.Entity
	}
	state.Entity = key
	emit := func(kind, reason string) error {
		id, err := watch.Revision(struct {
			Watch, Revision, Entity, Type string
			Sequence                      int64
		}{e.definition.Metadata.ID, e.revision, key, kind, o.Sequence})
		if err != nil {
			return err
		}
		message := reason
		if e.definition.Spec.Message != "" && (kind == "firing" || kind == "recovered") {
			var buf bytes.Buffer
			value := o.Fields[e.definition.Spec.Condition.Field]
			context := map[string]any{"Watch": e.definition.Metadata.ID, "Name": e.definition.Metadata.Name, "Type": kind, "Value": value, "Fields": o.Fields}
			if err := e.message.Execute(messageWriter{&buf}, context); err != nil {
				return fmt.Errorf("message template execution failed: %w", err)
			}
			if buf.Len() > 64*1024 {
				return fmt.Errorf("rendered message exceeds limit")
			}
			message = buf.String()
		}
		// Retained event fields must not alias mutable input data.
		data, err := json.Marshal(o.Fields)
		if err != nil {
			return err
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		var evidence []int64
		seen := map[int64]bool{}
		add := func(seq int64) {
			if !seen[seq] {
				evidence = append(evidence, seq)
				seen[seq] = true
			}
		}
		if kind == "firing" || kind == "recovered" {
			if e.definition.Spec.Condition.MissingFor != "" && state.FreshSequence > 0 {
				add(state.FreshSequence)
			}
			for _, sample := range state.Samples {
				add(sample.Sequence)
			}
		}
		for _, seq := range state.Evidence {
			add(seq)
		}
		add(o.Sequence)
		sort.Slice(evidence, func(i, j int) bool { return evidence[i] < evidence[j] })
		result.Events = append(result.Events, watch.Event{ID: id, WatchID: e.definition.Metadata.ID, Revision: e.revision, Entity: key, Type: kind, At: now, Message: message, Fields: fields, Evidence: evidence})
		return nil
	}
	unknown := func(reason string) (Transition, error) {
		state.Matches = 0
		state.Recoveries = 0
		state.Evidence = nil
		result.Reason = reason
		if !state.SourceUnhealthy {
			if err := emit("source_error", reason); err != nil {
				return result, err
			}
			state.SourceUnhealthy = true
			state.HealthEvent = result.Events[len(result.Events)-1].ID
		}
		state.LastSequence = o.Sequence
		if now.After(state.LastAt) {
			state.LastAt = now
		}
		result.State = state
		return result, nil
	}
	if !state.LastAt.IsZero() && now.Before(state.LastAt) {
		return unknown("clock_moved_backwards")
	}
	if o.Health == "gap" {
		state.Matches = 0
		state.Recoveries = 0
		state.Evidence = nil
		state.LastAt = now
		state.LastSequence = o.Sequence
		if err := emit("gap", "sampling_gap"); err != nil {
			return result, err
		}
		result.State = state
		result.Reason = "sampling_gap"
		return result, nil
	}
	if e.definition.Spec.Condition.MissingFor == "" && !state.LastAt.IsZero() && e.maxGap > 0 && now.Sub(state.LastAt) > e.maxGap {
		state.Matches = 0
		state.Recoveries = 0
		state.Evidence = nil
		if err := emit("gap", "sampling_gap"); err != nil {
			return result, err
		}
	}
	if o.Health == "unchanged" && e.definition.Spec.Condition.MissingFor == "" {
		// A 304 is evidence of source freshness, not another independent sample.
		state.LastAt = now
		state.LastSequence = o.Sequence
		result.State = state
		result.Reason = "unchanged"
		return result, nil
	}
	if o.Health != "ok" && !(e.definition.Spec.Condition.MissingFor != "" && (o.Health == "timer" || o.Health == "unchanged")) {
		return unknown(o.DetailOrDefault())
	}
	for k, want := range e.definition.Spec.Match {
		if o.Health == "timer" || o.Health == "unchanged" {
			break
		}
		actual, present := o.Fields[k]
		known, matched := Compare(actual, "eq", want)
		if !present || !known || !matched {
			state.LastAt = now
			state.LastSequence = o.Sequence
			result.State = state
			result.Reason = "filtered"
			return result, nil
		}
	}
	known, matched, reason := e.predicate(&state, o, now)
	if !known {
		return unknown(reason)
	}
	if state.SourceUnhealthy && o.Health != "timer" {
		if err := emit("source_recovered", "source_recovered"); err != nil {
			return result, err
		}
		state.SourceUnhealthy = false
		state.HealthEvent = ""
	}
	result.Known = true
	result.Matched = matched
	policy := e.definition.Spec.Policy
	if matched {
		state.Recoveries = 0
		if state.Matches < policy.Consecutive {
			state.Matches++
			state.Evidence = append(state.Evidence, o.Sequence)
		}
		if state.Matches >= policy.Consecutive {
			eligible := !state.Open
			if policy.Trigger == "level" {
				eligible = state.LastFired.IsZero() || !now.Before(state.LastFired.Add(e.interval))
			}
			if eligible {
				if err := emit("firing", "condition_matched"); err != nil {
					return result, err
				}
				state.LastFired = now
				state.IncidentEvent = result.Events[len(result.Events)-1].ID
			}
			state.Open = true
		}
	} else {
		state.Matches = 0
		if state.Open {
			if state.Recoveries == 0 {
				state.Evidence = nil
			}
			state.Recoveries++
			state.Evidence = append(state.Evidence, o.Sequence)
			if state.Recoveries >= policy.RecoverAfter {
				if err := emit("recovered", "condition_recovered"); err != nil {
					return result, err
				}
				state.Open = false
				state.IncidentEvent = ""
				state.Recoveries = 0
				state.Evidence = nil
			}
		} else {
			state.Recoveries = 0
			state.Evidence = nil
		}
	}
	state.LastAt = now
	state.LastSequence = o.Sequence
	result.State = state
	return result, nil
}
func (e *Evaluator) predicate(state *State, o watch.Observation, now time.Time) (bool, bool, string) {
	c := e.definition.Spec.Condition
	if c.MissingFor != "" {
		if o.Health == "timer" {
			if state.MissingAt.IsZero() && o.Deadline != nil {
				state.MissingAt = *o.Deadline
			}
			if state.MissingAt.IsZero() {
				return false, false, "missing_deadline"
			}
			if o.Deadline != nil && !state.MissingAt.Equal(*o.Deadline) {
				return false, false, "stale_deadline"
			}
			return true, !now.Before(state.MissingAt), ""
		}
		duration, _ := time.ParseDuration(c.MissingFor)
		state.MissingAt = now.Add(duration)
		state.FreshSequence = o.Sequence
		return true, false, ""
	}
	value, present := o.Fields[c.Field]
	if !present {
		return false, false, "missing_field"
	}
	if e.expression == nil {
		known, matched := Compare(value, c.Operator, c.Value)
		return known, matched, "wrong_field_type"
	}
	number, ok := Number(value)
	if !ok {
		return false, false, "wrong_field_type"
	}
	cutoff := now.Add(-e.maxWindow)
	retained := state.Samples[:0]
	for _, sample := range state.Samples {
		if sample.At.After(cutoff) && !sample.At.After(now) {
			retained = append(retained, sample)
		}
	}
	state.Samples = retained
	if e.maxWindow > 0 {
		if len(state.Samples) >= e.definition.Spec.Limits.MaxSamples {
			until := now.Add(e.maxWindow)
			if until.After(state.OverflowUntil) {
				state.OverflowUntil = until
			}
			return false, false, "window_sample_budget"
		}
		state.Samples = append(state.Samples, Sample{now, number, o.Sequence})
	}
	if now.Before(state.OverflowUntil) {
		return false, false, "window_sample_budget"
	}
	state.OverflowUntil = time.Time{}
	ctx := Context{Value: number, Aggregates: map[int]float64{}, Available: map[int]bool{}}
	for _, leaf := range e.expression.Windows() {
		var values []float64
		for _, sample := range state.Samples {
			if sample.At.After(now.Add(-leaf.Window)) && !sample.At.After(now) {
				values = append(values, sample.Value)
			}
		}
		if len(values) == 0 {
			return false, false, "empty_window"
		}
		aggregate := Aggregate(leaf.Func, values)
		if math.IsInf(aggregate, 0) || math.IsNaN(aggregate) {
			return false, false, "numeric_overflow"
		}
		ctx.Aggregates[leaf.ID] = aggregate
		ctx.Available[leaf.ID] = true
	}
	return true, e.expression.Eval(ctx), ""
}
func Number(v any) (float64, bool) {
	var n float64
	switch v := v.(type) {
	case float64:
		n = v
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case json.Number:
		var err error
		n, err = v.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
func Compare(actual any, op string, want any) (bool, bool) {
	if expected, ok := Number(want); ok {
		n, valid := Number(actual)
		if !valid {
			return false, false
		}
		operator := map[string]string{"eq": "==", "ne": "!=", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[op]
		if operator == "" {
			return false, false
		}
		return true, CompareNumber(n, operator, expected)
	}
	if op != "eq" && op != "ne" {
		return false, false
	}
	if want != nil && reflect.TypeOf(actual) != reflect.TypeOf(want) {
		return false, false
	}
	equal := reflect.DeepEqual(actual, want)
	if op == "ne" {
		equal = !equal
	}
	return true, equal
}
func Aggregate(function string, values []float64) float64 {
	if len(values) == 0 || (function != "count" && function != "avg" && function != "sum" && function != "min" && function != "max") {
		return math.NaN()
	}
	if function == "count" {
		return float64(len(values))
	}
	result := values[0]
	for _, value := range values[1:] {
		switch function {
		case "avg", "sum":
			result += value
		case "min":
			result = math.Min(result, value)
		case "max":
			result = math.Max(result, value)
		default:
			return math.NaN()
		}
	}
	if function == "avg" {
		result /= float64(len(values))
	}
	return result
}

// Limit allocation during template execution, including nested ranges.
type messageWriter struct{ buffer *bytes.Buffer }

func (w messageWriter) Write(p []byte) (int, error) {
	if w.buffer.Len()+len(p) > 64*1024 {
		return 0, fmt.Errorf("rendered message exceeds limit")
	}
	return w.buffer.Write(p)
}
