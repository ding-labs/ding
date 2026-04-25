package ingester_test

import (
	"strings"
	"testing"
	"time"

	"github.com/zuchka/ding/internal/ingester"
)

func TestParseJSONLine_Basic(t *testing.T) {
	events, err := ingester.ParseJSONLine([]byte(`{"metric":"cpu_usage","value":92.5,"host":"web-01"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Metric != "cpu_usage" {
		t.Errorf("expected metric cpu_usage, got %s", e.Metric)
	}
	if e.Value != 92.5 {
		t.Errorf("expected value 92.5, got %f", e.Value)
	}
	if e.Labels["host"] != "web-01" {
		t.Errorf("expected host web-01, got %s", e.Labels["host"])
	}
	if _, ok := e.Labels["timestamp"]; ok {
		t.Error("timestamp should not appear in labels")
	}
}

func TestParseJSONLine_TimestampOverride(t *testing.T) {
	events, err := ingester.ParseJSONLine([]byte(`{"metric":"cpu_usage","value":92.5,"timestamp":1711111111.5}`))
	if err != nil {
		t.Fatal(err)
	}
	e := events[0]
	expected := time.Unix(1711111111, 500000000)
	if !e.At.Equal(expected) {
		t.Errorf("expected timestamp %v, got %v", expected, e.At)
	}
}

func TestParseJSONLine_MissingMetric(t *testing.T) {
	_, err := ingester.ParseJSONLine([]byte(`{"value":92.5}`))
	if err == nil {
		t.Fatal("expected error for missing metric")
	}
}

func TestParseJSONLine_InvalidJSON(t *testing.T) {
	_, err := ingester.ParseJSONLine([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParsePrometheusText_Basic(t *testing.T) {
	input := strings.Join([]string{
		`# HELP cpu_usage Current CPU usage`,
		`# TYPE cpu_usage gauge`,
		`cpu_usage{host="web-01"} 92.5`,
		`http_errors{host="api-02",region="us-east"} 5`,
	}, "\n")

	events, err := ingester.ParsePrometheusText([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Metric != "cpu_usage" || events[0].Value != 92.5 {
		t.Errorf("unexpected first event: %+v", events[0])
	}
	if events[0].Labels["host"] != "web-01" {
		t.Errorf("expected host web-01, got %s", events[0].Labels["host"])
	}
	if events[1].Labels["region"] != "us-east" {
		t.Errorf("expected region us-east, got %s", events[1].Labels["region"])
	}
}

func TestParsePrometheusText_NoLabels(t *testing.T) {
	events, err := ingester.ParsePrometheusText([]byte("cpu_usage 42.0"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Value != 42.0 {
		t.Errorf("unexpected events: %+v", events)
	}
}

func TestParseJSONLine_NumericFieldGoesToFloats(t *testing.T) {
	events, err := ingester.ParseJSONLine([]byte(`{"metric":"price_tick","value":1.0,"price":77504.37,"exchange":"kraken"}`))
	if err != nil {
		t.Fatal(err)
	}
	e := events[0]

	// Numeric field must be in Floats, not Labels.
	if e.Floats == nil {
		t.Fatal("expected Floats map to be non-nil")
	}
	got, ok := e.Floats["price"]
	if !ok {
		t.Fatal("expected Floats[\"price\"] to be set")
	}
	if got != 77504.37 {
		t.Errorf("expected Floats[\"price\"] = 77504.37, got %v", got)
	}
	if _, inLabels := e.Labels["price"]; inLabels {
		t.Error("numeric field \"price\" must not appear in Labels")
	}

	// String field still goes to Labels.
	if e.Labels["exchange"] != "kraken" {
		t.Errorf("expected Labels[\"exchange\"] = \"kraken\", got %q", e.Labels["exchange"])
	}
	if _, inFloats := e.Floats["exchange"]; inFloats {
		t.Error("string field \"exchange\" must not appear in Floats")
	}
}

func TestParseJSONLine_NoNumericFields_FloatsNil(t *testing.T) {
	events, err := ingester.ParseJSONLine([]byte(`{"metric":"cpu_usage","value":92.5,"host":"web-01"}`))
	if err != nil {
		t.Fatal(err)
	}
	e := events[0]
	if e.Floats != nil {
		t.Errorf("expected Floats to be nil when no numeric extra fields, got %v", e.Floats)
	}
}

func TestParseJSONLine_MultipleNumericFields(t *testing.T) {
	events, err := ingester.ParseJSONLine([]byte(`{"metric":"m","value":1,"price":100.5,"qty":3.0,"label":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	e := events[0]
	if e.Floats["price"] != 100.5 {
		t.Errorf("expected price=100.5, got %v", e.Floats["price"])
	}
	if e.Floats["qty"] != 3.0 {
		t.Errorf("expected qty=3.0, got %v", e.Floats["qty"])
	}
	if e.Labels["label"] != "x" {
		t.Errorf("expected label=x, got %q", e.Labels["label"])
	}
	if len(e.Floats) != 2 {
		t.Errorf("expected 2 entries in Floats, got %d", len(e.Floats))
	}
}

func TestDetectFormat_JSONByContentType(t *testing.T) {
	format := ingester.DetectFormat(nil, "application/json", "auto")
	if format != "json" {
		t.Errorf("expected json, got %s", format)
	}
}

func TestDetectFormat_PrometheusHeuristic(t *testing.T) {
	data := []byte(`cpu_usage{host="web-01"} 92.5`)
	format := ingester.DetectFormat(data, "", "auto")
	if format != "prometheus" {
		t.Errorf("expected prometheus, got %s", format)
	}
}

func TestDetectFormat_JSONHeuristic(t *testing.T) {
	data := []byte(`{"metric":"cpu","value":1}`)
	format := ingester.DetectFormat(data, "", "auto")
	if format != "json" {
		t.Errorf("expected json, got %s", format)
	}
}

func TestDetectFormat_ServerFormatOverrides(t *testing.T) {
	format := ingester.DetectFormat(nil, "application/json", "prometheus")
	if format != "prometheus" {
		t.Errorf("expected prometheus (server override), got %s", format)
	}
}
