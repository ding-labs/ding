package notifier

import (
	"fmt"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
	"strings"
	"time"
)

type teamsTextBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Weight   string `json:"weight,omitempty"`
	Size     string `json:"size,omitempty"`
	Wrap     bool   `json:"wrap,omitempty"`
	IsSubtle bool   `json:"isSubtle,omitempty"`
}

type teamsFact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

type teamsFactSet struct {
	Type  string      `json:"type"`
	Facts []teamsFact `json:"facts"`
}

type teamsCard struct {
	Schema  string        `json:"$schema"`
	Type    string        `json:"type"`
	Version string        `json:"version"`
	Body    []interface{} `json:"body"`
}

type teamsAttachment struct {
	ContentType string    `json:"contentType"`
	ContentURL  *string   `json:"contentUrl"`
	Content     teamsCard `json:"content"`
}

type teamsMessage struct {
	Type        string            `json:"type"`
	Attachments []teamsAttachment `json:"attachments"`
}

type TeamsNotifier struct{ *httpNotifier }

func NewTeamsNotifier(url string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *TeamsNotifier {
	return &TeamsNotifier{httpNotifier: newHTTPNotifier(url, "teams", maxAttempts, initialBackoff, collector)}
}
func (n *TeamsNotifier) Send(alert evaluator.Alert) error {
	return n.send(alert.Rule, buildTeamsPayload(alert))
}
func buildTeamsPayload(alert evaluator.Alert) teamsMessage {
	body := []interface{}{
		teamsTextBlock{
			Type:   "TextBlock",
			Text:   alert.Rule,
			Weight: "Bolder",
			Size:   "Medium",
		},
	}

	if alert.Message != "" {
		body = append(body, teamsTextBlock{
			Type: "TextBlock",
			Text: alert.Message,
			Wrap: true,
		})
	}

	facts := []teamsFact{
		{Title: "Metric", Value: fmt.Sprintf("`%s`", alert.Metric)},
		{Title: "Value", Value: fmt.Sprintf("`%g`", alert.Value)},
	}

	if ec, ok := alert.Floats["exit_code"]; ok {
		facts = append(facts, teamsFact{
			Title: "exit code",
			Value: fmt.Sprintf("`%d`", int(ec)),
		})
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		facts = append(facts, teamsFact{
			Title: "duration",
			Value: fmt.Sprintf("`%.1fs`", dur),
		})
	}

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
		facts = append(facts, teamsFact{
			Title: label,
			Value: fmt.Sprintf("`%s`", display),
		})
	}

	body = append(body, teamsFactSet{
		Type:  "FactSet",
		Facts: facts,
	})

	body = append(body, teamsTextBlock{
		Type:     "TextBlock",
		Text:     fmt.Sprintf("Fired at %s", alert.FiredAt.Format(time.RFC3339)),
		IsSubtle: true,
		Size:     "Small",
	})

	card := teamsCard{
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Type:    "AdaptiveCard",
		Version: "1.5",
		Body:    body,
	}

	return teamsMessage{
		Type: "message",
		Attachments: []teamsAttachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				ContentURL:  nil,
				Content:     card,
			},
		},
	}
}
