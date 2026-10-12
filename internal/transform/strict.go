package transform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"unicode/utf8"
)

var ErrLimit = errors.New("JSON output exceeds configured limit")

// ValidateJSON bounds structural work before projection and rejects ambiguous
// keys and lossy integer identifiers. Body bytes are bounded by the caller too.
func ValidateJSON(ctx context.Context, raw []byte, maxDepth, maxNodes int) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("JSON must be UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	nodes := 0
	var value func(int) error
	value = func(depth int) error {
		nodes++
		if depth > maxDepth || nodes > maxNodes {
			return ErrLimit
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		tok, err := d.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON")
		}
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return fmt.Errorf("invalid object key")
					}
					name, ok := key.(string)
					if !ok || seen[name] {
						return fmt.Errorf("duplicate or invalid object key")
					}
					seen[name] = true
					if err := value(depth + 1); err != nil {
						return err
					}
				}
				end, err := d.Token()
				if err != nil || end != json.Delim('}') {
					return fmt.Errorf("invalid object")
				}
			case '[':
				for d.More() {
					if err := value(depth + 1); err != nil {
						return err
					}
				}
				end, err := d.Token()
				if err != nil || end != json.Delim(']') {
					return fmt.Errorf("invalid array")
				}
			default:
				return fmt.Errorf("unexpected delimiter")
			}
		case json.Number:
			f, err := v.Float64()
			if err != nil || math.IsInf(f, 0) || math.IsNaN(f) || (math.Trunc(f) == f && math.Abs(f) > 9007199254740991) {
				return fmt.Errorf("number outside supported range; encode large identifiers as strings")
			}
		}
		return nil
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return fmt.Errorf("JSON input must be an object")
	}
	if err := value(1); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	return nil
}
