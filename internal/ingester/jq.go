package ingester

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/itchyny/gojq"
)

// CompileJQ compiles a JQ expression. Returns an error if the expression is invalid.
// Call this once at startup; the returned *gojq.Code is safe for concurrent use.
func CompileJQ(expr string) (*gojq.Code, error) {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("parsing jq expression: %w", err)
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return nil, fmt.Errorf("compiling jq expression: %w", err)
	}
	return code, nil
}

// RunJQ runs a compiled JQ program against raw JSON bytes and returns the resulting events.
// The program may yield a single object or multiple objects (e.g. via .events[]).
// gojq returns an iterator; RunJQ drains it by calling iter.Next() in a loop until (nil, false).
// Each yielded map[string]interface{} is re-marshalled to JSON bytes and passed to ParseJSONLine,
// keeping ParseJSONLine's interface unchanged.
// Returns an error if:
//   - the input is not valid JSON
//   - the iterator yields a JQ runtime error
//   - a yielded value is nil (JQ null)
//   - a yielded value is a non-object type (string, number, etc.)
//   - no values are yielded (empty result)
func RunJQ(code *gojq.Code, rawBytes []byte) ([]Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	return RunJQContext(ctx, code, rawBytes, 100, 1<<20)
}

func RunJQContext(ctx context.Context, code *gojq.Code, rawBytes []byte, maxOutputs, maxBytes int) ([]Event, error) {
	if maxOutputs <= 0 || maxBytes <= 0 || len(rawBytes) > maxBytes {
		return nil, fmt.Errorf("jq input or limit exceeds budget")
	}
	var input interface{}
	if err := json.Unmarshal(rawBytes, &input); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	iter := code.RunWithContext(ctx, input)
	outputBytes := 0
	var events []Event
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return nil, fmt.Errorf("jq runtime error: %w", err)
		}
		if v == nil {
			return nil, fmt.Errorf("jq produced null output")
		}
		if len(events) >= maxOutputs {
			return nil, fmt.Errorf("jq output count exceeds budget")
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("jq produced unexpected output type: %T", v)
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("jq output marshal error: %w", err)
		}
		outputBytes += len(b)
		if outputBytes > maxBytes {
			return nil, fmt.Errorf("jq output bytes exceed budget")
		}
		evs, err := ParseJSONLine(b)
		if err != nil {
			return nil, err
		}
		events = append(events, evs...)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("jq produced no output")
	}
	return events, nil
}
