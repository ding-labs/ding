package replay

import (
	"math"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
)

// Evaluation describes only a replayed checkpoint, never current live state.
type Evaluation struct {
	Known           bool            `json:"known"`
	Matched         bool            `json:"matched"`
	Reason          string          `json:"reason"`
	Matches         int             `json:"matches"`
	Recoveries      int             `json:"recoveries"`
	Open            bool            `json:"open"`
	SourceUnhealthy bool            `json:"sourceUnhealthy"`
	Windows         []WindowSummary `json:"windows"`
}
type WindowSummary struct {
	Function         string    `json:"function"`
	FromExclusive    time.Time `json:"fromExclusive"`
	ThroughInclusive time.Time `json:"throughInclusive"`
	Samples          int       `json:"samples"`
	Value            *float64  `json:"value"`
	Available        bool      `json:"available"`
}

func Explain(proof Evidence) (*Evaluation, error) {
	if proof.Checkpoint == nil {
		return nil, nil
	}
	c := proof.Checkpoint
	if c.SourceRecovered {
		return &Evaluation{Reason: "source_recovered", Windows: []WindowSummary{}}, nil
	}
	p, err := plan.Compile(proof.Definition)
	if err != nil {
		return nil, err
	}
	e, err := condition.New(p.Definition, p.Revision)
	if err != nil {
		return nil, err
	}
	r, err := e.Evaluate(c.Prior, c.Input, c.Input.AcceptedAt)
	if err != nil {
		return nil, err
	}
	out := &Evaluation{Known: r.Known, Matched: r.Matched, Reason: r.Reason, Matches: r.State.Matches, Recoveries: r.State.Recoveries, Open: r.State.Open, SourceUnhealthy: r.State.SourceUnhealthy, Windows: []WindowSummary{}}
	if p.Definition.Spec.Condition.Numeric != "" {
		expression, err := condition.ParseExpression(p.Definition.Spec.Condition.Numeric)
		if err != nil {
			return nil, err
		}
		for _, leaf := range expression.Windows() {
			w := WindowSummary{Function: leaf.Func, FromExclusive: c.Input.AcceptedAt.Add(-leaf.Window), ThroughInclusive: c.Input.AcceptedAt}
			values := []float64{}
			for _, s := range r.State.Samples {
				if s.At.After(w.FromExclusive) && !s.At.After(w.ThroughInclusive) {
					values = append(values, s.Value)
				}
			}
			w.Samples = len(values)
			if r.Known && len(values) > 0 {
				v := condition.Aggregate(leaf.Func, values)
				if !math.IsNaN(v) && !math.IsInf(v, 0) {
					w.Value = &v
					w.Available = true
				}
			}
			out.Windows = append(out.Windows, w)
		}
	}
	return out, nil
}
