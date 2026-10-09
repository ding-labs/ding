package delivery

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ding-labs/ding/internal/watch"
)

func TestPayloadFormatsAndBounds(t *testing.T) {
	e := watch.Event{ID: "stable", WatchID: "api", Type: "firing", At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Message: strings.Repeat("🚨", 2000) + "<!channel> @everyone", Fields: map[string]any{"http.status": 503}}
	for _, kind := range []string{"webhook", "console", "slack", "discord"} {
		data, err := Render(kind, e)
		if err != nil || !utf8.Valid(data) || !json.Valid(data) {
			t.Fatal(kind, err)
		}
		var m map[string]any
		json.Unmarshal(data, &m)
		switch kind {
		case "slack":
			if len(data) > 4000 || m["mrkdwn"] != false {
				t.Fatal(string(data))
			}
			blocks := m["blocks"].([]any)
			text := blocks[1].(map[string]any)["text"].(map[string]any)
			if text["type"] != "plain_text" {
				t.Fatal("source content could become Slack markup")
			}
		case "discord":
			mentions := m["allowed_mentions"].(map[string]any)["parse"].([]any)
			if len(mentions) != 0 || len(data) > 4000 {
				t.Fatal(string(data))
			}
		default:
			if m["apiVersion"] != watch.APIVersion {
				t.Fatal(m)
			}
		}
	}
	e.Type = "recovered"
	if _, err := Render("discord", e); err != nil {
		t.Fatal(err)
	}
	if _, err := Render("unsupported", e); err == nil {
		t.Fatal("unsupported accepted")
	}
	e.Fields["invalid"] = make(chan int)
	if _, err := Render("slack", e); err == nil {
		t.Fatal("invalid value accepted")
	}
}
