package cloud

import (
	"bytes"
	"context"
	"maps"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestHostedTypedServicePreview(t *testing.T) {
	f := newHostedMCPFixture(t)
	var requests atomic.Int32
	transport := f.s.Pool.http.Transport
	f.s.Pool.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return transport.RoundTrip(r)
	})
	tenant, err := f.s.Pool.Get(context.Background(), f.session.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: reusedDestination}); err != nil {
		t.Fatal(err)
	}
	token := f.token("owner", "client", "ding:inspect ding:preview ding:manage", "")
	args := map[string]any{"watch_id": "starter", "url": "https://example.com/health", "destination_ref": "shared-alerts", "workspace_id": f.session.Workspace}
	var handle any
	for _, status := range []int{0, 204} {
		request := maps.Clone(args)
		if status != 0 {
			request["expected_status"] = status
		}
		view := hostedView(t, f.call(t, token, "ding_preview_service_watch", request))
		if view.Target == nil || view.Target.Workspace != f.session.Workspace || view.Target.Execution != "cloud" {
			t.Fatal("wrong target", view.Target)
		}
		data := view.Data.(map[string]any)
		manifest, ok := data["manifest"].(string)
		if !ok || data["valid"] != true || len(data["descriptions"].([]any)) != 1 {
			t.Fatal("incomplete starter preview", data)
		}
		bundle, err := plan.Parse([]byte(manifest))
		if err != nil || len(bundle.Watches) != 1 || len(bundle.Destinations) != 0 {
			t.Fatal("invalid generated recipe", err)
		}
		spec := bundle.Watches[0].Definition.Spec
		cadence, _ := time.ParseDuration(spec.Source.Every)
		if cadence != 5*time.Minute || spec.Source.Timeout != "5s" || spec.Limits.MaxBytes != 1<<20 || spec.Policy.Consecutive != 3 || spec.Policy.RecoverAfter != 2 || spec.Destinations[0].Ref != "shared-alerts" {
			t.Fatal("wrong recipe", spec)
		}
		operator, value := "gte", "500"
		if status != 0 {
			operator, value = "ne", "204"
		}
		if spec.Condition.Operator != operator || string(mustJSON(t, spec.Condition.Value)) != value {
			t.Fatal("expected status lost", spec.Condition)
		}
		handle = data["preview"].(map[string]any)["handle"]
	}
	for _, change := range []map[string]any{
		{"expected_status": 600}, {"expected_status": 204.5}, {"expected_status": nil},
		{"destination_ref": "missing"}, {"url": "http://127.0.0.1/health"},
		{"url": "http://10.1.2.3/"}, {"every": "1s"}, {"workspace_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	} {
		bad := maps.Clone(args)
		maps.Copy(bad, change)
		if result := f.call(t, token, "ding_preview_service_watch", bad); !result.IsError {
			t.Fatal("invalid preview accepted", change)
		}
	}
	read := f.token("owner", "client", "ding:inspect", "")
	if result := f.call(t, read, "ding_preview_service_watch", args); !result.IsError || result.Meta["mcp/www_authenticate"] == nil {
		t.Fatal("preview scope not enforced", result)
	}
	watches, err := tenant.App.List(context.Background())
	if err != nil || len(watches) != 0 || requests.Load() != 0 {
		t.Fatal("preview executed I/O or created a watch", err, requests.Load())
	}
	apply := map[string]any{"handle": handle, "operation_key": "typed_service_apply_0001", "workspace_id": f.session.Workspace}
	first := hostedView(t, f.call(t, token, "ding_apply_changes", apply))
	again := hostedView(t, f.call(t, token, "ding_apply_changes", apply))
	if !bytes.Equal(mustJSON(t, first.Data), mustJSON(t, again.Data)) {
		t.Fatal("apply did not reconcile its receipt")
	}
	if result := f.call(t, token, "ding_preview_service_watch", args); !result.IsError {
		t.Fatal("starter would replace an existing watch")
	}
}
