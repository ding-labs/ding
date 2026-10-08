package notifier

import (
	"fmt"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
	"strings"
	"time"
)

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields"`
	Footer      *discordFooter `json:"footer,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

type discordMessage struct {
	Embeds []discordEmbed `json:"embeds"`
}

type DiscordNotifier struct{ *httpNotifier }

func NewDiscordNotifier(url string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *DiscordNotifier {
	return &DiscordNotifier{httpNotifier: newHTTPNotifier(url, "discord", maxAttempts, initialBackoff, collector)}
}
func (n *DiscordNotifier) Send(alert evaluator.Alert) error {
	return n.send(alert.Rule, buildDiscordPayload(alert))
}
func buildDiscordPayload(alert evaluator.Alert) discordMessage {
	embed := discordEmbed{
		Title: alert.Rule,
		Color: 0xE74C3C, // red
	}

	if alert.Message != "" {
		embed.Description = alert.Message
	}

	// Build fields: metric/value first, then priority floats (exit_code, duration),
	// then label fields. Discord allows up to 25 fields per embed.
	fields := []discordField{
		{Name: "Metric", Value: fmt.Sprintf("`%s`", alert.Metric), Inline: true},
		{Name: "Value", Value: fmt.Sprintf("`%g`", alert.Value), Inline: true},
	}

	if ec, ok := alert.Floats["exit_code"]; ok {
		fields = append(fields, discordField{
			Name:   "exit code",
			Value:  fmt.Sprintf("`%d`", int(ec)),
			Inline: true,
		})
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		fields = append(fields, discordField{
			Name:   "duration",
			Value:  fmt.Sprintf("`%.1fs`", dur),
			Inline: true,
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
		fields = append(fields, discordField{
			Name:   label,
			Value:  fmt.Sprintf("`%s`", display),
			Inline: true,
		})
	}

	embed.Fields = fields
	embed.Footer = &discordFooter{
		Text: fmt.Sprintf("Fired at %s", alert.FiredAt.Format(time.RFC3339)),
	}

	return discordMessage{Embeds: []discordEmbed{embed}}
}
