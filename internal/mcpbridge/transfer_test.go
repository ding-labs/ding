package mcpbridge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func transferFixture(t *testing.T) (*Bridge, map[string]*watchrun.App, *atomic.Int32) {
	t.Helper()
	ctx := context.Background()
	apps := map[string]*watchrun.App{}
	backends := map[string]backend{}
	sent := &atomic.Int32{}
	for _, id := range []string{"local", "cloud"} {
		db, err := store.Open(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		app := watchrun.New(db)
		apps[id] = app
		app.Lookup = func(string) (string, bool) { return "https://hooks.example.com/test", true }
		app.HTTP.Client = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
			if r.Method == "POST" {
				sent.Add(1)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
		})}
		g, err := app.CreateIntegrationGrant(ctx, watchrun.GrantRequest{Name: "bridge", Days: 1, Scopes: []string{"inspect", "preview", "manage", "move"}, SecretRefs: []string{"WEBHOOK"}})
		if err != nil {
			t.Fatal(err)
		}
		host := httptest.NewServer(control.Handler(app, control.Credentials{Admin: strings.Repeat("a", 64)}))
		t.Cleanup(host.Close)
		client := mcpclient.New(mcpconfig.Connection{DaemonURL: host.URL, Token: g.Token, GrantID: g.Grant.ID})
		t.Cleanup(client.Close)
		backends[id] = local(client)
	}
	manifest := `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: hook}
spec: {type: webhook, urlRef: {env: WEBHOOK}}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: same-watch}
spec:
  source: {type: http, url: https://example.com, every: 5m, timeout: 2s}
  condition: {field: http.status, operator: gte, value: 500}
  policy: {consecutive: 1, recoverAfter: 1}
  limits: {maxBytes: 65536, maxEntities: 10, maxSamples: 100, maxOutputs: 10}
  destinations: [{ref: hook, events: [firing, recovered]}]
`
	if _, err := apps["local"].Apply(ctx, watchrun.ApplyRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	observe(t, apps["local"])
	return &Bridge{journal: t.TempDir(), backends: backends}, apps, sent
}

func observe(t *testing.T, app *watchrun.App) {
	t.Helper()
	ctx := context.Background()
	r, err := app.Record(ctx, "same-watch")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 200}}}}, fmt.Sprint(time.Now().UnixNano()), time.Now()); err != nil {
		t.Fatal(err)
	}
}
func reviewed(t *testing.T, app *watchrun.App, from, to, key string) map[string]any {
	t.Helper()
	p, err := app.PreflightHandoff(context.Background(), "same-watch")
	if err != nil || !p.Ready {
		t.Fatal(p, err)
	}
	return map[string]any{"source_connection": from, "target_connection": to, "watch_id": "same-watch", "expected_revision": p.Watch.Plan.Revision, "expected_generation": p.Watch.Generation, "expected_destinations": p.Destinations, "operation_key": key, "upload_confirmed": true}
}
func runTransfer(t *testing.T, b *Bridge, name string, args map[string]any) map[string]any {
	t.Helper()
	v, err := b.transfer(context.Background(), name, args)
	if err != nil {
		t.Fatal(name, err)
	}
	return v.Data.(map[string]any)
}

func TestReviewedTransferLostActivationRestartAndMoveBack(t *testing.T) {
	b, apps, sent := transferFixture(t)
	ctx := context.Background()
	args := reviewed(t, apps["local"], "local", "cloud", "prepare_transfer_0001")
	prepared := runTransfer(t, b, "ding_prepare_transfer", args)
	id := prepared["id"].(string)
	runTransfer(t, b, "ding_prepare_transfer", args)
	for name, want := range map[string]string{"local": "running", "cloud": "paused"} {
		r, _ := apps[name].Record(ctx, "same-watch")
		if r.Status != want {
			t.Fatal("preparation changed execution", name, r.Status)
		}
	}
	if _, err := b.transfer(ctx, "ding_finish_transfer", map[string]any{"transfer_id": id, "delivery_confirmed": true}); err == nil {
		t.Fatal("finished without a test")
	}
	if sent.Load() != 0 {
		t.Fatal("preparation sent test automatically")
	}
	testArgs := map[string]any{"transfer_id": id, "operation_key": "test_transfer_000001"}
	for range 2 {
		if tested := runTransfer(t, b, "ding_test_transfer", testArgs); tested["outcome"] != "accepted" {
			t.Fatal(tested)
		}
	}
	if sent.Load() != 1 {
		t.Fatal("same-key test sent twice", sent.Load())
	}
	real := b.backends["cloud"]
	lost := false
	b.backends["cloud"] = func(ctx context.Context, fn func(mcpcontract.Target, call) (mcpcontract.View, error)) (mcpcontract.View, error) {
		return real(ctx, func(target mcpcontract.Target, invoke call) (mcpcontract.View, error) {
			return fn(target, func(ctx context.Context, name string, a map[string]any) (mcpcontract.View, error) {
				v, err := invoke(ctx, name, a)
				if err == nil && name == "ding_bridge_transfer_step" && a["action"] == "activate" && !lost {
					lost = true
					return v, mcpclient.Unknown()
				}
				return v, err
			})
		})
	}
	finish := map[string]any{"transfer_id": id, "delivery_confirmed": true}
	if _, err := b.transfer(ctx, "ding_finish_transfer", finish); err == nil {
		t.Fatal("lost response reported success")
	}
	for name, want := range map[string]string{"local": "paused", "cloud": "running"} {
		r, _ := apps[name].Record(ctx, "same-watch")
		if r.Status != want {
			t.Fatal("lost activation duplicated execution", name, r.Status)
		}
	}
	restarted := &Bridge{journal: b.journal, backends: b.backends}
	if result := runTransfer(t, restarted, "ding_finish_transfer", finish); result["phase"] != "active_waiting_observation" {
		t.Fatal(result)
	}
	if sent.Load() != 1 {
		t.Fatal("recovery repeated external test")
	}
	observe(t, apps["cloud"])
	if status := runTransfer(t, restarted, "ding_get_transfer_status", map[string]any{"transfer_id": id}); status["firstObservationConfirmed"] != true {
		t.Fatal(status)
	}
	if _, err := restarted.transfer(ctx, "ding_cancel_transfer", map[string]any{"transfer_id": id, "cancel_confirmed": true}); err == nil {
		t.Fatal("active target canceled")
	}
	back := runTransfer(t, restarted, "ding_prepare_transfer", reviewed(t, apps["cloud"], "cloud", "local", "prepare_return_00001"))["id"].(string)
	runTransfer(t, restarted, "ding_test_transfer", map[string]any{"transfer_id": back, "operation_key": "return_test_0000001"})
	runTransfer(t, restarted, "ding_finish_transfer", map[string]any{"transfer_id": back, "delivery_confirmed": true})
	for name, want := range map[string]string{"local": "running", "cloud": "paused"} {
		r, _ := apps[name].Record(ctx, "same-watch")
		if r.Status != want {
			t.Fatal("reverse transfer duplicated execution", name, r.Status)
		}
	}
}

func TestIneligibleReviewDoesNotConnectAndCancelDoesNotPause(t *testing.T) {
	b, apps, _ := transferFixture(t)
	ctx := context.Background()
	real := b.backends["cloud"]
	connected := false
	b.backends["cloud"] = func(ctx context.Context, fn func(mcpcontract.Target, call) (mcpcontract.View, error)) (mcpcontract.View, error) {
		connected = true
		return real(ctx, fn)
	}
	args := reviewed(t, apps["local"], "local", "cloud", "prepare_cancel_00001")
	args["expected_generation"] = 999
	if _, err := b.transfer(ctx, "ding_prepare_transfer", args); err == nil || connected {
		t.Fatal("stale source connected to cloud")
	}
	args = reviewed(t, apps["local"], "local", "cloud", "prepare_cancel_00001")
	id := runTransfer(t, b, "ding_prepare_transfer", args)["id"].(string)
	for range 2 {
		runTransfer(t, b, "ding_cancel_transfer", map[string]any{"transfer_id": id, "cancel_confirmed": true})
	}
	r, _ := apps["local"].Record(ctx, "same-watch")
	if r.Status != "running" {
		t.Fatal("cancellation paused source")
	}
	r, _ = apps["cloud"].Record(ctx, "same-watch")
	if r.Status != "deleted" {
		t.Fatal("canceled target remained executable")
	}
}
