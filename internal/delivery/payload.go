package delivery

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ding-labs/ding/internal/notify"
	"github.com/ding-labs/ding/internal/watch"
)

// Render is pure; payloads are pinned before the outbox transaction commits.
func Render(kind string, event watch.Event) ([]byte, error) {
	if kind == "webhook" || kind == "console" {
		return json.Marshal(watch.Envelope{APIVersion: watch.APIVersion, Data: event})
	}
	title := truncate(event.WatchID+" · "+event.Type, 150)
	if kind == "desktop" {
		return json.Marshal(notify.Message{ID: event.ID, WatchID: event.WatchID, Title: title, Body: truncate(event.Message, 2800)})
	}
	text := event.Message
	keys := make([]string, 0, len(event.Fields))
	for key := range event.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for i, key := range keys {
		if i >= 10 {
			break
		}
		value, err := json.Marshal(event.Fields[key])
		if err != nil {
			return nil, err
		}
		text += "\n" + truncate(key, 128) + ": " + truncate(string(value), 160)
	}
	text = truncate(strings.TrimSpace(text), 2800)
	if text == "" {
		text = event.Type
	}
	footer := event.At.Format(time.RFC3339) + " · " + event.ID
	switch kind {
	case "slack":
		plain := func(text string) map[string]any { return map[string]any{"type": "plain_text", "text": text} }
		return json.Marshal(map[string]any{"text": truncate(title, 150), "mrkdwn": false, "unfurl_links": false, "unfurl_media": false, "blocks": []any{map[string]any{"type": "header", "text": plain(title)}, map[string]any{"type": "section", "text": plain(text)}, map[string]any{"type": "context", "elements": []any{plain(footer)}}}})
	case "discord":
		color := 0xe74c3c
		if strings.Contains(event.Type, "recovered") {
			color = 0x2ecc71
		}
		return json.Marshal(map[string]any{"allowed_mentions": map[string]any{"parse": []string{}}, "embeds": []any{map[string]any{"title": title, "description": text, "color": color, "footer": map[string]any{"text": footer}}}})
	default:
		return nil, fmt.Errorf("unsupported destination type")
	}
}
func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	limit := max - 3
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit] + "…"
}
