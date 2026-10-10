package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestCloudHandoffRequiresTestsAndRejectsCredentialChanges(t *testing.T) {
	ctx := context.Background()
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	tenant, err := s.Pool.Get(ctx, sessions[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	var deliveries atomic.Int64
	tenant.App.HTTP.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			deliveries.Add(1)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}
	if err := s.Vault.Put(ctx, tenant.Account.ID, "WEBHOOK", "https://example.com/hook"); err != nil {
		t.Fatal(err)
	}
	localDB, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer localDB.Close()
	local := watchrun.New(localDB)
	if _, err := local.Apply(ctx, watchrun.ApplyRequest{Manifest: fixture}); err != nil {
		t.Fatal(err)
	}
	record, _ := local.Record(ctx, "health")
	_, err = local.Accept(ctx, record, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 200}}}}, "first", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := local.PreflightHandoff(ctx, "health")
	if err != nil || !preflight.Ready {
		t.Fatal(preflight, err)
	}
	call := func(path string, body any, account int) (int, []byte) {
		raw, _ := json.Marshal(body)
		w := cloudRequest(h, "POST", path, string(raw), sessions[account], true)
		return w.Code, w.Body.Bytes()
	}
	var preview struct {
		Data watchrun.ApplyResult `json:"data"`
	}
	code, raw := call("/v1/apply", watchrun.ApplyRequest{Manifest: preflight.Manifest, DryRun: true}, 0)
	if code != 200 || json.Unmarshal(raw, &preview) != nil {
		t.Fatal(string(raw))
	}
	id := state.ID()
	code, raw = call("/v1/handoffs/prepare", watchrun.HandoffPrepare{ID: id, Peer: preflight.Instance, Manifest: preflight.Manifest, Review: preview.Data.Review}, 0)
	var prepared struct {
		Data store.Handoff `json:"data"`
	}
	if code != 200 || json.Unmarshal(raw, &prepared) != nil {
		t.Fatal(code, string(raw))
	}
	if code, _ := call("/v1/handoffs/"+id+"/activate", store.Handoff{}, 0); code != 409 {
		t.Fatal("untested target activated")
	}
	if code, _ := call("/v1/handoffs/"+id+"/test", map[string]string{"operationKey": state.ID()}, 1); code != 409 {
		t.Fatal("cross-tenant transfer test accepted")
	}
	key := state.ID()
	for i := 0; i < 2; i++ {
		code, raw = call("/v1/handoffs/"+id+"/test", map[string]string{"operationKey": key}, 0)
		if code != 200 || !strings.Contains(string(raw), "accepted") {
			t.Fatal(code, string(raw))
		}
	}
	if deliveries.Load() != 1 {
		t.Fatal("transfer test sent twice")
	}
	proof, err := local.PauseForHandoff(ctx, watchrun.HandoffPause{ID: id, WatchID: "health", Peer: prepared.Data.Instance, Revision: record.Plan.Revision, Generation: record.Generation, Destinations: preflight.Destinations})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Vault.Put(ctx, tenant.Account.ID, "WEBHOOK", "https://example.com/replaced"); err != nil {
		t.Fatal(err)
	}
	if code, _ := call("/v1/handoffs/"+id+"/activate", proof, 0); code != 409 {
		t.Fatal("changed credential proof accepted")
	}
	code, raw = call("/v1/handoffs/"+id+"/test", map[string]string{"operationKey": state.ID()}, 0)
	if code != 200 {
		t.Fatal(string(raw))
	}
	code, raw = call("/v1/handoffs/"+id+"/activate", proof, 0)
	if code != 200 {
		t.Fatal(code, string(raw))
	}
	code, raw = call("/v1/handoffs/"+id+"/activate", proof, 0)
	if code != 200 {
		t.Fatal("activation replay", string(raw))
	}
	if _, err := local.Lifecycle(ctx, "health", watchrun.LifecycleRequest{Action: "resume"}); err == nil {
		t.Fatal("local resumed while cloud active")
	}
}
