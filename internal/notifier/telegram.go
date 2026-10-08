package notifier

import (
	"fmt"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
	"strings"
	"time"
)

type TelegramNotifier struct {
	*httpNotifier
	chatID string
}

func NewTelegramNotifier(token, chatID string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *TelegramNotifier {
	return NewTelegramNotifierAt("https://api.telegram.org", token, chatID, maxAttempts, initialBackoff, collector)
}
func NewTelegramNotifierAt(apiBase, token, chatID string, maxAttempts int, initialBackoff time.Duration, collector *metrics.Collector) *TelegramNotifier {
	return &TelegramNotifier{httpNotifier: newHTTPNotifier(apiBase+"/bot"+token+"/sendMessage", "telegram", maxAttempts, initialBackoff, collector), chatID: chatID}
}
func (n *TelegramNotifier) Send(alert evaluator.Alert) error {
	return n.send(alert.Rule, map[string]string{"chat_id": n.chatID, "parse_mode": "HTML", "text": buildTelegramMessage(alert)})
}
func buildTelegramMessage(alert evaluator.Alert) string {
	var b strings.Builder

	fmt.Fprintf(&b, "<b>%s</b>", alert.Rule)

	if alert.Message != "" {
		fmt.Fprintf(&b, "\n\n%s", alert.Message)
	}

	b.WriteString("\n\n")
	fmt.Fprintf(&b, "<b>metric</b>  <code>%s</code>\n", alert.Metric)
	fmt.Fprintf(&b, "<b>value</b>  <code>%g</code>", alert.Value)

	if ec, ok := alert.Floats["exit_code"]; ok {
		fmt.Fprintf(&b, "\n<b>exit code</b>  <code>%d</code>", int(ec))
	}
	if dur, ok := alert.Floats["duration_seconds"]; ok {
		fmt.Fprintf(&b, "\n<b>duration</b>  <code>%.1fs</code>", dur)
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
		fmt.Fprintf(&b, "\n<b>%s</b>  <code>%s</code>", label, display)
	}

	fmt.Fprintf(&b, "\n\n<i>%s</i>", alert.FiredAt.Format(time.RFC3339))

	return b.String()
}
