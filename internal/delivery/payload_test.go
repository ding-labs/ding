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

func TestProviderNotificationBells(t *testing.T) {
	for _, tc := range []struct {
		kind, severity, label, icon string
		color                       int
	}{
		{"firing", "", "Error", "red", 0xe77e7e},
		{"firing", "warning", "Warning", "yellow", 0xe7e37e},
		{"source_error", "warning", "Error", "red", 0xe77e7e},
		{"gap", "", "Warning", "yellow", 0xe7e37e},
		{"recovered", "error", "Recovered", "green", 0x7ee787},
		{"source_recovered", "", "Recovered", "green", 0x7ee787},
		{"new-event", "error", "Error", "red", 0xe77e7e},
		{"changed", "", "", "", 0},
		{"new-event", "info", "", "", 0},
		{"test", "", "", "", 0},
	} {
		for _, provider := range []string{"slack", "discord"} {
			t.Run(provider+"/"+tc.kind+"/"+tc.severity, func(t *testing.T) {
				event := watch.Event{ID: "stable", WatchID: "api", Type: tc.kind, Message: "<!channel> @everyone", Fields: map[string]any{"severity": tc.severity}}
				data, err := Render(provider, event)
				if err != nil {
					t.Fatal(err)
				}
				var payload map[string]any
				if err := json.Unmarshal(data, &payload); err != nil {
					t.Fatal(err)
				}
				wantURL := "https://ding.ing/assets/notifications/" + tc.icon + ".png"
				var title string
				if provider == "slack" {
					title = payload["text"].(string)
					section := payload["blocks"].([]any)[1].(map[string]any)
					if tc.icon == "" {
						if section["accessory"] != nil {
							t.Fatal("neutral event acquired a status bell")
						}
					} else {
						image := section["accessory"].(map[string]any)
						if image["type"] != "image" || image["image_url"] != wantURL || image["alt_text"] != "Ding: "+tc.label {
							t.Fatal(image)
						}
					}
					if payload["icon_url"] != nil || payload["icon_emoji"] != nil {
						t.Fatal("incoming webhooks cannot change Slack's app avatar")
					}
				} else {
					embed := payload["embeds"].([]any)[0].(map[string]any)
					title = embed["title"].(string)
					if tc.icon == "" {
						if embed["thumbnail"] != nil || embed["color"] != nil {
							t.Fatal("neutral event acquired a health status")
						}
					} else if embed["thumbnail"].(map[string]any)["url"] != wantURL || embed["color"] != float64(tc.color) {
						t.Fatal(embed)
					}
				}
				if tc.label != "" && !strings.HasPrefix(title, tc.label+" · ") {
					t.Fatal("missing accessible status label", title)
				}
			})
		}
	}
}

func TestNotificationStylingPreservesMachineEventPayloads(t *testing.T) {
	event := watch.Event{ID: "stable", WatchID: "api", Type: "firing", Fields: map[string]any{"severity": "warning"}}
	want, err := json.Marshal(watch.Envelope{APIVersion: watch.APIVersion, Data: event})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"console", "webhook"} {
		got, err := Render(kind, event)
		if err != nil || string(got) != string(want) {
			t.Fatalf("%s changed event evidence: %s, %v", kind, got, err)
		}
	}
}
