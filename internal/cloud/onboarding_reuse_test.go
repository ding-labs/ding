package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watchrun"
)

const reusedDestination = `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: shared-alerts}
spec: {type: webhook, urlRef: {env: WEBHOOK}}
`

func TestStarterTemplatesReferenceWithoutRedefiningDestination(t *testing.T) {
	local, _, err := onboarding.Manifest(onboarding.Request{ID: "local", URL: "https://example.com/health", DestinationRef: "shared-alerts"})
	if err != nil {
		t.Fatal(err)
	}
	hosted, err := Template(FirstWatch{ID: "hosted", URL: "https://example.com/health", DestinationRef: "shared-alerts"})
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range []string{local, hosted} {
		b, err := plan.Parse([]byte(manifest))
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Destinations) != 0 || b.Watches[0].Definition.Spec.Destinations[0].Ref != "shared-alerts" {
			t.Fatal("reference redefined destination", manifest)
		}
	}
	if _, _, err := onboarding.Manifest(onboarding.Request{Delivery: "console", DestinationRef: "shared-alerts"}); err == nil {
		t.Fatal("accepted ambiguous local delivery")
	}
	if _, err := Template(FirstWatch{Credential: "OTHER", DestinationRef: "shared-alerts"}); err == nil {
		t.Fatal("accepted ambiguous hosted delivery")
	}
}

func TestHostedOnboardingReuseIsTenantBoundAndRevisionReviewed(t *testing.T) {
	s, handler, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	tenant, err := s.Pool.Get(context.Background(), sessions[0].Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: reusedDestination}); err != nil {
		t.Fatal(err)
	}
	body := `{"id":"second-watch","url":"https://example.com/health","destinationRef":"shared-alerts"}`
	preview := cloudRequest(handler, "POST", "/v1/onboarding/preview", body, sessions[0], true)
	var result struct {
		Data control.FirstWatchPreview `json:"data"`
	}
	if preview.Code != 200 || json.Unmarshal(preview.Body.Bytes(), &result) != nil {
		t.Fatal(preview.Code, preview.Body.String())
	}
	if len(result.Data.Review.DestinationChanges) != 0 || result.Data.Review.Review.Destinations["shared-alerts"] == "" {
		t.Fatal("destination revision not reviewed", result)
	}
	if other := cloudRequest(handler, "POST", "/v1/onboarding/preview", body, sessions[1], true); other.Code != 400 {
		t.Fatal("cross-tenant destination reused", other.Code)
	}
	if _, err := tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: strings.ReplaceAll(reusedDestination, "WEBHOOK", "CHANGED_WEBHOOK")}); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(watchrun.ApplyRequest{Manifest: result.Data.Manifest, Review: result.Data.Review.Review})
	if stale := cloudRequest(handler, "POST", "/v1/apply", string(request), sessions[0], true); stale.Code != 409 {
		t.Fatal("accepted stale destination", stale.Code)
	}
	preview = cloudRequest(handler, "POST", "/v1/onboarding/preview", body, sessions[0], true)
	if preview.Code != 200 || json.Unmarshal(preview.Body.Bytes(), &result) != nil {
		t.Fatal(preview.Code)
	}
	request, _ = json.Marshal(watchrun.ApplyRequest{Manifest: result.Data.Manifest, Review: result.Data.Review.Review})
	if applied := cloudRequest(handler, "POST", "/v1/apply", string(request), sessions[0], true); applied.Code != 200 {
		t.Fatal(applied.Code, applied.Body.String())
	}
}
