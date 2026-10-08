package notifier

import (
	"fmt"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
	"sort"
	"time"
)

type PagerDutyNotifier struct {
	*httpNotifier
	routingKey string
}

const pagerDutyEventsV2URL = "https://events.pagerduty.com/v2/enqueue"

func NewPagerDutyNotifier(routingKey string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *PagerDutyNotifier {
	return NewPagerDutyNotifierAt(routingKey, maxAttempts, initialBackoff, collector, pagerDutyEventsV2URL)
}
func NewPagerDutyNotifierAt(routingKey string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector, endpoint string) *PagerDutyNotifier {
	return &PagerDutyNotifier{httpNotifier: newHTTPNotifier(endpoint, "pagerduty", maxAttempts, initialBackoff, collector), routingKey: routingKey}
}
func (n *PagerDutyNotifier) Send(alert evaluator.Alert) error {
	return n.send(alert.Rule, buildPagerDutyPayload(alert, n.routingKey))
}

type pdPayload struct {
	RoutingKey  string      `json:"routing_key"`
	EventAction string      `json:"event_action"`
	DedupKey    string      `json:"dedup_key"`
	Payload     pdEventBody `json:"payload"`
}

type pdEventBody struct {
	Summary       string                 `json:"summary"`
	Source        string                 `json:"source"`
	Severity      string                 `json:"severity"`
	Timestamp     string                 `json:"timestamp"`
	CustomDetails map[string]interface{} `json:"custom_details,omitempty"`
}

func buildPagerDutyPayload(alert evaluator.Alert, routingKey string) pdPayload {
	summary := alert.Rule
	if alert.Message != "" {
		summary = alert.Rule + ": " + alert.Message
	}
	if len(summary) > 1024 {
		summary = summary[:1024]
	}

	source := alert.Rule
	if v := alert.Labels["repo"]; v != "" {
		source = v
	} else if v := alert.Labels["workflow"]; v != "" {
		source = v
	}

	cd := make(map[string]interface{})
	cd["metric"] = alert.Metric
	cd["value"] = fmt.Sprintf("%g", alert.Value)
	if ec, ok := alert.Floats["exit_code"]; ok {
		cd["exit_code"] = fmt.Sprintf("%g", ec)
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		cd["duration_seconds"] = fmt.Sprintf("%g", dur)
	}
	keys := make([]string, 0, len(alert.Labels))
	for k := range alert.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cd[k] = alert.Labels[k]
	}

	return pdPayload{
		RoutingKey:  routingKey,
		EventAction: "trigger",
		DedupKey:    alert.Rule,
		Payload: pdEventBody{
			Summary:       summary,
			Source:        source,
			Severity:      "critical",
			Timestamp:     alert.FiredAt.UTC().Format(time.RFC3339),
			CustomDetails: cd,
		},
	}
}
