package cloud

import (
	"context"
	"encoding/json"
	"github.com/ding-labs/ding/internal/watchrun"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDestinationTestReconcilesWithoutSendingTwice(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	tenant, err := s.Pool.Get(context.Background(), sessions[0].Workspace)
	if err != nil {
		t.Fatal(err)
	}
	var sent atomic.Int64
	tenant.App.HTTP.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		sent.Add(1)
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "not an incident") {
			t.Error("test was not labeled")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}
	if err := s.Vault.Put(context.Background(), tenant.Workspace.ID, "WEBHOOK", "https://example.com/hook"); err != nil {
		t.Fatal(err)
	}
	body := `{"operationKey":"a-stable-test-operation","destination":"webhook","credential":"WEBHOOK"}`
	for i := 0; i < 2; i++ {
		w := cloudRequest(h, "POST", "/v1/cloud/destination-test", body, sessions[0], true)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "accepted") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if sent.Load() != 1 {
		t.Fatal("test notification duplicated")
	}
	if err := s.Vault.Put(context.Background(), tenant.Workspace.ID, "WEBHOOK", "https://example.com/changed"); err != nil {
		t.Fatal(err)
	}
	if w := cloudRequest(h, "POST", "/v1/cloud/destination-test", body, sessions[0], true); w.Code != 409 {
		t.Fatal("changed credential reused proof", w.Code)
	}
}

func TestSavedDestinationTestIsTenantScopedAndUsesReviewedRevision(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	tenant, err := s.Pool.Get(context.Background(), sessions[0].Workspace)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: reusedDestination})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Vault.Put(context.Background(), tenant.Workspace.ID, "WEBHOOK", "https://example.com/test-secret"); err != nil {
		t.Fatal(err)
	}
	var sent atomic.Int64
	tenant.App.HTTP.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		sent.Add(1)
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	request := watchrun.DestinationTestRequest{OperationKey: "saved-destination-test-01", DestinationRef: "shared-alerts", Revision: result.DestinationChanges[0].Revision}
	body, _ := json.Marshal(request)
	for range 2 {
		w := cloudRequest(h, "POST", "/v1/cloud/destination-test", string(body), sessions[0], true)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "accepted") || strings.Contains(w.Body.String(), "test-secret") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := cloudRequest(h, "POST", "/v1/cloud/destination-test", string(body), sessions[1], true); w.Code != 404 {
		t.Fatal("cross-tenant test", w.Code)
	}
	request.Revision = "stale"
	body, _ = json.Marshal(request)
	if w := cloudRequest(h, "POST", "/v1/cloud/destination-test", string(body), sessions[0], true); w.Code != 409 {
		t.Fatal("stale test", w.Code)
	}
	if sent.Load() != 1 {
		t.Fatal("unexpected send count", sent.Load())
	}
}
