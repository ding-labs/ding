package ingester

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// ParseJSONLine parses a single JSON object line into one Event.
func ParseJSONLine(data []byte) ([]Event, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	metricRaw, ok := raw["metric"]
	if !ok {
		return nil, fmt.Errorf("missing required field \"metric\"")
	}
	metric, ok := metricRaw.(string)
	if !ok || metric == "" {
		return nil, fmt.Errorf("\"metric\" must be a non-empty string")
	}

	valueRaw, ok := raw["value"]
	if !ok {
		return nil, fmt.Errorf("missing required field \"value\"")
	}
	value, ok := toFloat64(valueRaw)
	if !ok {
		return nil, fmt.Errorf("\"value\" must be a number")
	}

	at := time.Now()
	if tsRaw, ok := raw["timestamp"]; ok {
		switch v := tsRaw.(type) {
		case string:
			parsed, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return nil, fmt.Errorf("timestamp must be RFC3339 or Unix seconds")
			}
			at = parsed
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) || v < -62135596800 || v >= 253402300800 {
				return nil, fmt.Errorf("timestamp outside supported range")
			}
			sec := int64(v)
			at = time.Unix(sec, int64((v-float64(sec))*1e9))
		default:
			return nil, fmt.Errorf("timestamp must be RFC3339 or Unix seconds")
		}
	}

	labels := make(map[string]string)
	var floats map[string]float64
	for k, v := range raw {
		if k == "metric" || k == "value" || k == "timestamp" {
			continue
		}
		switch tv := v.(type) {
		case string:
			labels[k] = tv
		case float64:
			if floats == nil {
				floats = make(map[string]float64)
			}
			floats[k] = tv
			// bool, nil, nested objects/arrays: skip
		}
	}

	return []Event{{Metric: metric, Value: value, Labels: labels, Floats: floats, At: at}}, nil
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}
