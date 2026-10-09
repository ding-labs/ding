package replay

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

// Checkpoint reproduces one entity transition. It is committed with the event,
// before maintenance can discard unrelated observations or change live state.
type Checkpoint struct {
	Prior           condition.State   `json:"prior"`
	Input           watch.Observation `json:"input"`
	SourceRecovered bool              `json:"sourceRecovered,omitempty"`
}
type Evidence struct {
	Evaluation   *Evaluation      `json:"evaluation,omitempty"`
	Definition   watch.Definition `json:"definition"`
	Event        watch.Event      `json:"event"`
	Checkpoint   *Checkpoint      `json:"checkpoint,omitempty"`
	ReplayStatus string           `json:"replayStatus"`
}

// Verify has no clock, source, credential or delivery dependency. Selected
// evidence alone is not a full history: the checkpoint supplies preceding state.
func Verify(proof Evidence) error {
	if proof.Checkpoint == nil {
		return fmt.Errorf("event has no replay checkpoint")
	}
	p, err := plan.Compile(proof.Definition)
	if err != nil {
		return err
	}
	if p.Revision != proof.Event.Revision || p.Definition.Metadata.ID != proof.Event.WatchID {
		return fmt.Errorf("evidence definition mismatch")
	}
	e, err := condition.New(p.Definition, p.Revision)
	if err != nil {
		return err
	}
	c := proof.Checkpoint
	var events []watch.Event
	if c.SourceRecovered {
		if c.Prior.Entity != condition.SourceEntity || !c.Prior.SourceUnhealthy {
			return fmt.Errorf("invalid source recovery checkpoint")
		}
		keys, clear, err := e.Route([]string{condition.SourceEntity}, c.Input)
		if err != nil || !clear || len(keys) == 0 {
			return fmt.Errorf("invalid source recovery input")
		}
		events = []watch.Event{e.SourceRecovered(c.Input)}
	} else {
		result, err := e.Evaluate(c.Prior, c.Input, c.Input.AcceptedAt)
		if err != nil {
			return err
		}
		events = result.Events
	}
	want := proof.Event
	want.Sequence = 0
	expected, err := json.Marshal(want)
	if err != nil {
		return err
	}
	for _, event := range events {
		event.Sequence = 0
		got, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if bytes.Equal(got, expected) {
			return nil
		}
	}
	return fmt.Errorf("replay did not reproduce the recorded event")
}
