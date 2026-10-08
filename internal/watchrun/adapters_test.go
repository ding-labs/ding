package watchrun

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/watch"
)

func TestProviderPayloadAndAcknowledgment(t *testing.T) {
	for _, provider := range []string{"slack", "discord"} {
		t.Run(provider, func(t *testing.T) {
			a, _ := setup(t)
			m := strings.Replace(manifest("https://example.com"), "type: webhook", "type: "+provider, 1)
			if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
				t.Fatal(err)
			}
			record, _ := a.Record(ctx, "api")
			for n := 1; n <= 3; n++ {
				input(t, a, record, n, 500)
			}
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if provider == "slack" {
					if payload["blocks"] == nil {
						t.Error("missing blocks")
					}
					w.Write([]byte("rejected"))
				} else {
					if payload["allowed_mentions"] == nil {
						t.Error("mentions not controlled")
					}
					w.WriteHeader(204)
				}
			}))
			defer receiver.Close()
			a.Lookup = func(string) (string, bool) { return receiver.URL, true }
			a.Now = func() time.Time { return start.Add(20 * time.Second) }
			if _, err := a.DeliverOne(ctx); err != nil {
				t.Fatal(err)
			}
			i, _ := a.Inspect(ctx, "api")
			want := "delivered"
			if provider == "slack" {
				want = "permanent"
			}
			if i.Deliveries[0].Status != want {
				t.Fatal(i.Deliveries[0])
			}
		})
	}
}
func TestSignalStateSurvivesRestartAndRetention(t *testing.T) {
	for _, operator := range []string{"changed", "new-event"} {
		t.Run(operator, func(t *testing.T) {
			a, _ := setup(t)
			condition := "field: http.status, operator: changed"
			if operator == "new-event" {
				condition = "field: http.status, operator: new-event, dedupFor: 72h"
			}
			m := strings.Replace(manifest("https://example.com"), "field: http.status, operator: gte, value: 500", condition, 1)
			m = strings.Replace(m, "consecutive: 3, recoverAfter: 2", "trigger: level, interval: 0s", 1)
			m = strings.Replace(m, "  condition:", "  limits: {idleTTL: 1h}\n  condition:", 1)
			if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
				t.Fatal(err)
			}
			record, _ := a.Record(ctx, "api")
			batch := source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": "a"}}}}
			if _, err := a.Accept(ctx, record, batch, "first", start); err != nil {
				t.Fatal(err)
			}
			if err := a.Maintain(ctx, start.Add(48*time.Hour)); err != nil {
				t.Fatal(err)
			}
			i, _ := a.Inspect(ctx, "api")
			if len(i.Entities) != 1 {
				t.Fatal("expired required signal state")
			}
			if _, err := a.Accept(ctx, record, batch, "same", start.Add(48*time.Hour)); err != nil {
				t.Fatal(err)
			}
			s := state(t, a)
			if operator == "new-event" && len(s.Seen) != 1 {
				t.Fatal(s)
			}
			if operator == "changed" && len(s.Baseline) == 0 {
				t.Fatal(s)
			}
		})
	}
}
