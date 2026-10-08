package source

import (
	"context"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

// Acquirer returns a batch for a single scheduled acquisition. The caller owns
// acceptance time, transactions, checkpoints, retries, and overlap prevention.
type Acquirer interface {
	Fetch(context.Context, plan.Compiled, string, time.Time) Batch
}

var _ Acquirer = HTTP{}
var _ Acquirer = Command{}

func Observations(outputs []map[string]any, observedAtField string) ([]watch.Observation, error) {
	observations := []watch.Observation{}
	for _, fields := range outputs {
		o := watch.Observation{Health: "ok", Fields: fields}
		if observedAtField != "" {
			text, ok := fields[observedAtField].(string)
			if !ok {
				return nil, fmt.Errorf("observed time must be an RFC3339 string")
			}
			instant, err := time.Parse(time.RFC3339Nano, text)
			if err != nil {
				return nil, fmt.Errorf("observed time must be an RFC3339 string")
			}
			o.ObservedAt = &instant
		}
		observations = append(observations, o)
	}
	if len(observations) == 0 {
		observations = append(observations, watch.Observation{Health: "unchanged"})
	}
	return observations, nil
}
