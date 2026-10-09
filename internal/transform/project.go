// Package transform provides bounded, typed JSON projections without filesystem,
// environment, network, or command access from jq programs.
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/itchyny/gojq"
)

func CompileJQ(expression string) (*gojq.Code, error) {
	query, err := gojq.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression: %w", err)
	}
	return code, nil
}
func Scalar(value any) bool {
	switch v := value.(type) {
	case nil, bool:
		return true
	case string:
		return utf8.ValidString(v)
	case float64:
		return !math.IsNaN(v) && !math.IsInf(v, 0)
	case int, int64, json.Number:
		return true
	}
	return false
}
func Field(object any, path string) (any, bool) {
	value := object
	for _, part := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

// Project applies jq first, then explicit dotted-path field selection. Without
// selection, each output must already be a flat object of scalar fields.
func Project(ctx context.Context, raw []byte, expression string, fields map[string]string, maxOutputs, maxBytes int) ([]map[string]any, error) {
	if maxOutputs < 1 || maxBytes < 1 || len(raw) > maxBytes || !utf8.Valid(raw) {
		return nil, fmt.Errorf("transform input exceeds limits or is not UTF-8")
	}
	var input any
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("invalid source JSON")
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	var iterator gojq.Iter
	if expression != "" {
		code, err := CompileJQ(expression)
		if err != nil {
			return nil, err
		}
		iterator = code.RunWithContext(ctx, input)
	} else {
		iterator = gojq.NewIter(input)
	}
	outputs := []map[string]any{}
	used := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("transform deadline exceeded")
		}
		value, more := iterator.Next()
		if !more {
			break
		}
		if _, ok := value.(error); ok {
			return nil, fmt.Errorf("transform evaluation failed")
		}
		if len(outputs) >= maxOutputs {
			return nil, fmt.Errorf("transform output count exceeds limit")
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("transform output must be an object")
		}
		selected := map[string]any{}
		if len(fields) > 0 {
			for alias, path := range fields {
				if value, ok := Field(object, path); ok {
					if !Scalar(value) {
						return nil, fmt.Errorf("selected fields must be scalar")
					}
					selected[alias] = value
				}
			}
		} else {
			for name, value := range object {
				if !Scalar(value) {
					return nil, fmt.Errorf("output fields must be scalar; select nested fields explicitly")
				}
				selected[name] = value
			}
		}
		encoded, err := json.Marshal(selected)
		if err != nil {
			return nil, fmt.Errorf("invalid transform output")
		}
		used += len(encoded)
		if used > maxBytes {
			return nil, fmt.Errorf("transform output bytes exceed limit")
		}
		// Normalize gojq integer values back to the portable JSON number type.
		if err := json.Unmarshal(encoded, &selected); err != nil {
			return nil, err
		}
		outputs = append(outputs, selected)
	}
	return outputs, nil
}
