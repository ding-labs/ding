// Package identity encodes structured keys without delimiter or Unicode collisions.
package identity

import "encoding/json"

// Key preserves arbitrary bytes, including invalid UTF-8, in each component.
// JSON encodes byte slices as base64 and preserves component boundaries.
func Key(parts ...string) string {
	values := make([][]byte, len(parts))
	for i, part := range parts {
		values[i] = []byte(part)
	}
	b, _ := json.Marshal(values) // byte slices cannot fail to marshal
	return string(b)
}

// Parts decodes a key. Callers validate the component count for their key type.
func Parts(key string) ([]string, error) {
	var values [][]byte
	if err := json.Unmarshal([]byte(key), &values); err != nil {
		return nil, err
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = string(value)
	}
	return parts, nil
}
