package watchrun

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/replay"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func TestEvidenceReproducesFiringRecoveryAcrossRevisionRestartAndRetention(t *testing.T) {
	a, dir := setup(t)
	r := apply(t, a, "https://example.com")
	for n := 1; n <= 3; n++ {
		input(t, a, r, n, 500)
	}
	// Compatible display edit preserves incident state while event evidence pins
	// the original immutable definition.
	m := strings.Replace(manifest("https://example.com"), "metadata: {id: api}", "metadata: {id: api, name: Changed}", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	r, _ = a.Record(ctx, "api")
	input(t, a, r, 4, 200)
	input(t, a, r, 5, 200)
	a.Store.Close()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a = New(s)
	if err := a.Maintain(ctx, start.Add(8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var events []watch.Event
	s.View(ctx, func(tx *store.Tx) error { var err error; events, err = tx.Events("api", 0, 100); return err })
	found := 0
	for _, e := range events {
		if e.Type != "firing" && e.Type != "recovered" {
			continue
		}
		found++
		proof, err := a.Evidence(ctx, e.ID)
		if err != nil {
			t.Fatal(e.Type, err)
		}
		if proof.ReplayStatus != "verified" || proof.Checkpoint.Input.Sequence == 0 {
			t.Fatal(proof)
		}
		raw, _ := json.Marshal(proof)
		var copied replay.Evidence
		json.Unmarshal(raw, &copied)
		if err := replay.Verify(copied); err != nil {
			t.Fatal(err)
		}
		copied.Event.Message = "tampered"
		if replay.Verify(copied) == nil {
			t.Fatal("accepted changed event")
		}
	}
	if found != 2 {
		t.Fatal(events)
	}
}
func TestEvidenceForSignalTimerAndSourceRecovery(t *testing.T) {
	for _, kind := range []string{"changed", "new-event", "timer", "source"} {
		t.Run(kind, func(t *testing.T) {
			a, _ := setup(t)
			m := manifest("https://example.com")
			m = strings.Replace(m, "  destinations: [{ref: hook, events: [firing, recovered]}]", "", 1)
			switch kind {
			case "changed", "new-event":
				c := "field: http.status, operator: " + kind
				if kind == "new-event" {
					c += ", dedupFor: 1h"
				}
				m = strings.Replace(m, "field: http.status, operator: gte, value: 500", c, 1)
				m = strings.Replace(m, "consecutive: 3, recoverAfter: 2", "trigger: level, interval: 0s", 1)
			case "timer":
				m = strings.Replace(m, "field: http.status, operator: gte, value: 500", "missingFor: 10s", 1)
				m = strings.Replace(m, "consecutive: 3, recoverAfter: 2", "consecutive: 1, recoverAfter: 1", 1)
			case "source":
				m = strings.Replace(m, "  condition:", "  groupBy: [host]\n  condition:", 1)
			}
			if _, err := a.Apply(ctx, ApplyRequest{Manifest: m}); err != nil {
				t.Fatal(err)
			}
			r, _ := a.Record(ctx, "api")
			accept := func(id string, sec int, health string, fields map[string]any) {
				t.Helper()
				if _, err := a.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: health, Fields: fields}}}, id, start.Add(time.Duration(sec)*time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "timer":
				if err := a.RunTimers(ctx, start.Add(10*time.Second)); err != nil {
					t.Fatal(err)
				}
			case "source":
				accept("a", 1, "unknown", nil)
				accept("b", 2, "ok", map[string]any{"http.status": 200, "host": "a"})
			default:
				accept("a", 1, "ok", map[string]any{"http.status": "a"})
				accept("b", 2, "ok", map[string]any{"http.status": "b"})
			}
			var events []watch.Event
			a.Store.View(ctx, func(tx *store.Tx) error { events, _ = tx.Events("api", 0, 100); return nil })
			count := 0
			for _, event := range events {
				proof, err := a.Evidence(ctx, event.ID)
				if err != nil {
					t.Fatal(event.Type, err)
				}
				if proof.Checkpoint != nil {
					if kind == "new-event" && event.Type == "new-event" && len(proof.Checkpoint.Prior.Seen) != 0 {
						t.Fatal("unrelated IDs copied into firing checkpoint")
					}
					count++
					if replay.Verify(proof) != nil {
						t.Fatal(proof)
					}
				}
			}
			if count == 0 {
				t.Fatal("no replayed transitions", events)
			}
		})
	}
}
func TestExportAndDoctorNeverResolveSecretsIntoOutput(t *testing.T) {
	a, _ := setup(t)
	r := apply(t, a, "https://example.com")
	a.running = true
	secret := "https://example.com/private-token-abc"
	a.Lookup = func(string) (string, bool) { return secret, true }
	manifest, err := a.Export(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := plan.Parse([]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Destinations) != 1 || bundle.Watches[0].Revision != r.Plan.Revision || strings.Contains(manifest, secret) {
		t.Fatal(manifest)
	}
	d, err := a.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(d)
	if !d.Healthy || len(d.Credentials) != 1 || !d.Credentials[0].Present || strings.Contains(string(raw), secret) {
		t.Fatal(string(raw))
	}
	if len(r.Plan.Permissions) == 0 {
		t.Fatal("lost permissions")
	}
	a.Lookup = func(string) (string, bool) { return "", false }
	d, err = a.Doctor(ctx)
	if err != nil || d.Healthy {
		t.Fatal(d, err)
	}
}
