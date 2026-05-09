package evaluator

import (
	"fmt"
	"time"
)

// humanizeDuration converts a numeric seconds value to a human-readable
// duration string using Go's native time.Duration.String() format
// (e.g., 30m43s, 4m7.3s, 500ms). Non-numeric inputs pass through via
// fmt.Sprint so templates don't crash on bad data; users see something
// obviously wrong (e.g. "<nil>", "[1 2]") and can fix the template.
//
// Used as a text/template FuncMap entry. Pipe form in templates:
//
//	{{ .duration_seconds | humanize_duration }}
func humanizeDuration(seconds interface{}) string {
	var f float64
	switch v := seconds.(type) {
	case float64:
		f = v
	case float32:
		f = float64(v)
	case int:
		f = float64(v)
	case int32:
		f = float64(v)
	case int64:
		f = float64(v)
	case uint:
		f = float64(v)
	case uint32:
		f = float64(v)
	case uint64:
		f = float64(v)
	default:
		return fmt.Sprint(seconds)
	}
	return time.Duration(f * float64(time.Second)).String()
}

// defaultValue returns fallback when value is nil or the empty string;
// otherwise returns value unchanged. Intentionally narrower than sprig's
// default to avoid the "0/false fall back" footgun.
//
// Used as a text/template FuncMap entry. Pipe form in templates:
//
//	{{ .branch | default "unknown" }}
func defaultValue(fallback interface{}, value interface{}) interface{} {
	if value == nil {
		return fallback
	}
	if s, ok := value.(string); ok && s == "" {
		return fallback
	}
	return value
}
