package notifier

import (
	"fmt"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
	"strings"
	"time"
)

type slackTextObj struct {
	Type string `json:"type"` // "plain_text" or "mrkdwn"
	Text string `json:"text"`
}

type slackFieldObj struct {
	Type string `json:"type"` // "mrkdwn"
	Text string `json:"text"`
}

type slackBlock struct {
	Type     string           `json:"type"`
	Text     *slackTextObj    `json:"text,omitempty"`
	Fields   []*slackFieldObj `json:"fields,omitempty"`
	Elements []*slackTextObj  `json:"elements,omitempty"`
}

type slackMessage struct {
	Blocks []slackBlock `json:"blocks"`
}

type SlackNotifier struct{ *httpNotifier }

func NewSlackNotifier(url string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *SlackNotifier {
	return &SlackNotifier{httpNotifier: newHTTPNotifier(url, "slack", maxAttempts, initialBackoff, collector)}
}
func (n *SlackNotifier) Send(alert evaluator.Alert) error {
	return n.send(alert.Rule, buildSlackPayload(alert))
}
func buildSlackPayload(alert evaluator.Alert) slackMessage {
	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackTextObj{Type: "plain_text", Text: alert.Rule},
		},
	}

	if alert.Message != "" {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackTextObj{Type: "mrkdwn", Text: alert.Message},
		})
	}

	// Build fields: metric/value first, then priority floats (exit_code, duration),
	// then label fields. This ensures the most actionable job-context signals appear
	// even when many labels are present and the 10-field Slack limit is hit.
	fields := []*slackFieldObj{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Metric:* `%s`", alert.Metric)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Value:* `%g`", alert.Value)},
	}

	if ec, ok := alert.Floats["exit_code"]; ok {
		fields = append(fields, &slackFieldObj{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*exit code:* `%d`", int(ec)),
		})
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		fields = append(fields, &slackFieldObj{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*duration:* `%.1fs`", dur),
		})
	}

	// Run-context label fields — order is stable and user-facing.
	for _, key := range []string{"branch", "commit", "repo", "workflow", "job", "actor", "runner", "run_id"} {
		v, ok := alert.Labels[key]
		if !ok || v == "" {
			continue
		}
		display := v
		if key == "commit" && len(v) > 7 {
			display = v[:7]
		}
		label := strings.ReplaceAll(key, "_", " ")
		fields = append(fields, &slackFieldObj{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*%s:* `%s`", label, display),
		})
	}

	// Slack section blocks are limited to 10 fields.
	if len(fields) > 10 {
		fields = fields[:10]
	}

	blocks = append(blocks, slackBlock{
		Type:   "section",
		Fields: fields,
	})

	blocks = append(blocks, slackBlock{
		Type: "context",
		Elements: []*slackTextObj{
			{Type: "mrkdwn", Text: fmt.Sprintf("Fired at %s", alert.FiredAt.Format(time.RFC3339))},
		},
	})

	return slackMessage{Blocks: blocks}
}
