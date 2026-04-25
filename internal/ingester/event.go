package ingester

import "time"

// Event is a single parsed metric data point.
type Event struct {
	Metric string
	Value  float64
	Labels map[string]string  // never contains "timestamp"
	Floats map[string]float64 // extra numeric fields (non-metric, non-value, non-timestamp)
	At     time.Time          // ingestion wall clock, or overridden by JSON "timestamp"
}
