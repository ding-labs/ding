package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/demo"
	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

// Exercise both generated recipes through real HTTP acquisition, SQLite restart,
// evaluation, retained evidence, and HTTP delivery. The hosted recipe runs in a
// local test runtime; this does not qualify hosted egress or a deployed service.
func TestFirstWatchRecipesFailureTimeoutRestartAndRecovery(t *testing.T) {
	for _, hosted := range []bool{false, true} {
		t.Run(fmt.Sprintf("hosted=%t", hosted), func(t *testing.T) {
			ctx := context.Background()
			fixture := httptest.NewServer(demo.HealthFixture())
			defer fixture.Close()
			received := make(chan watch.Event, 10)
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload struct {
					Data watch.Event `json:"data"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				received <- payload.Data
				w.WriteHeader(http.StatusNoContent)
			}))
			defer receiver.Close()
			manifest, _, err := onboarding.Manifest(onboarding.Request{ID: "service", URL: "https://example.com/health", Delivery: "console"})
			if err != nil {
				t.Fatal(err)
			}
			manifest = strings.Replace(manifest, "type: console", "type: webhook\n  urlRef: {env: RECEIVER}", 1)
			if hosted {
				manifest, err = Template(FirstWatch{ID: "service", URL: "https://example.com/health", Destination: "webhook", Credential: "RECEIVER"})
				if err != nil {
					t.Fatal(err)
				}
			}
			manifest = strings.ReplaceAll(manifest, "https://example.com/health", fixture.URL+"/health")
			dir := t.TempDir()
			db, err := store.Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { db.Close() }()
			now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
			var app *watchrun.App
			configure := func() {
				app = watchrun.New(db)
				app.Now = func() time.Time { return now }
				app.Lookup = func(name string) (string, bool) { return receiver.URL, name == "RECEIVER" }
			}
			configure()
			if _, err := app.Apply(ctx, watchrun.ApplyRequest{Manifest: manifest}); err != nil {
				t.Fatal(err)
			}
			inspect := func() watchrun.Inspection {
				t.Helper()
				i, err := app.Inspect(ctx, "service")
				if err != nil {
					t.Fatal(err)
				}
				return i
			}
			if !inspect().Watch.LastInputAt.IsZero() {
				t.Fatal("apply claimed input before acquisition")
			}
			consecutive := func(open bool, matches int) {
				t.Helper()
				var state condition.State
				if err := json.Unmarshal(inspect().Entities["[]"], &state); err != nil {
					t.Fatal(err)
				}
				if state.Open != open || state.Matches != matches {
					t.Fatalf("unexpected state: %+v", state)
				}
			}
			restart := func() {
				t.Helper()
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				db, err = store.Open(ctx, dir)
				if err != nil {
					t.Fatal(err)
				}
				configure()
			}
			sequence := 0
			poll := func(mode string) {
				t.Helper()
				req, _ := http.NewRequest(http.MethodPut, fixture.URL+"/mode", strings.NewReader(mode))
				response, err := fixture.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 204 {
					t.Fatal(response.StatusCode)
				}
				record := inspect().Watch
				every, _ := time.ParseDuration(record.Plan.Definition.Spec.Source.Every)
				now = now.Add(every)
				// Shorten only the test's timeout context, preserving the recipe.
				fetchCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
				batch := app.HTTP.Fetch(fetchCtx, record.Plan, record.Cursor, now)
				sequence++
				if _, err := app.Accept(ctx, record, batch, fmt.Sprint(sequence), now); err != nil {
					t.Fatal(err)
				}
			}
			poll("healthy")
			consecutive(false, 0)
			poll("failing")
			poll("failing")
			consecutive(false, 2)
			restart()
			consecutive(false, 2)
			poll("failing")
			if !strings.Contains(string(inspect().Entities["[]"]), `"open":true`) {
				t.Fatal("three failures did not fire")
			}
			poll("timeout")
			restart()
			var afterTimeout condition.State
			if err := json.Unmarshal(inspect().Entities["[]"], &afterTimeout); err != nil {
				t.Fatal(err)
			}
			if !afterTimeout.Open || !afterTimeout.SourceUnhealthy {
				t.Fatal("timeout cleared incident or source error", afterTimeout)
			}
			poll("healthy")
			if !strings.Contains(string(inspect().Entities["[]"]), `"open":true`) {
				t.Fatal("recovered before two responses")
			}
			poll("healthy")
			consecutive(false, 0)
			for range 10 {
				worked, err := app.DeliverOne(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if !worked {
					break
				}
			}
			got := map[string]int{}
			for len(received) > 0 {
				event := <-received
				got[event.Type]++
				if event.Type == "firing" || event.Type == "recovered" {
					evidence, err := app.Evidence(ctx, event.ID)
					if err != nil || evidence.ReplayStatus != "verified" {
						t.Fatal("unverified evidence", evidence, err)
					}
				}
			}
			want := map[string]int{"firing": 1, "recovered": 1, "source_error": 1, "source_recovered": 1}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("received %v, want %v", got, want)
			}
			for _, intent := range inspect().Deliveries {
				if intent.Status != "delivered" {
					t.Fatal("undelivered notification", intent)
				}
			}
		})
	}
}
