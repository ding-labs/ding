package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/watchrun"
)

func TestProtectedDestinationSetupCreatesNoWatch(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	session := sessions[0]
	tenant, err := s.Pool.Get(context.Background(), session.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	sent := 0
	tenant.App.HTTP.Client = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		sent++
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	if err := s.Vault.Put(context.Background(), session.Workspace, "HOOK", "https://example.com/private-hook"); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"accountID": session.User, "workspaceID": session.Workspace, "id": "assistant-alerts", "type": "webhook", "credential": "HOOK"}
	body, _ := json.Marshal(payload)
	if w := cloudRequest(h, "POST", "/v1/onboarding/destination", string(body), session, false); w.Code != 401 {
		t.Fatal("CSRF accepted", w.Code)
	}
	for range 2 {
		w := cloudRequest(h, "POST", "/v1/onboarding/destination", string(body), session, true)
		if w.Code != 201 && w.Code != 409 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	read := func(want string) watchrun.DestinationSetupStatus {
		t.Helper()
		w := cloudRequest(h, "GET", "/v1/onboarding/destination-status?destination=assistant-alerts", "", session, false)
		var out struct {
			Data watchrun.DestinationSetupStatus `json:"data"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Data.State != want || strings.Contains(w.Body.String(), "private-hook") {
			t.Fatal(w.Code, w.Body.String())
		}
		return out.Data
	}
	status := read("untested")
	request := watchrun.DestinationTestRequest{AccountID: session.User, WorkspaceID: session.Workspace, DestinationRef: status.DestinationRef, Revision: status.Revision, OperationKey: "protected-setup-test-0001"}
	body, _ = json.Marshal(request)
	for range 2 {
		w := cloudRequest(h, "POST", "/v1/cloud/destination-test", string(body), session, true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	read("accepted")
	if w := cloudRequest(h, "POST", "/v1/onboarding/destination-confirm", string(body), sessions[1], true); w.Code != 403 {
		t.Fatal("cross-workspace confirmation", w.Code, w.Body.String())
	}
	if w := cloudRequest(h, "POST", "/v1/onboarding/destination-confirm", string(body), session, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	read("confirmed")
	// A URL's expected account is never permission to create in another account.
	payload["accountID"] = sessions[1].User
	payload["id"] = "wrong-account"
	body, _ = json.Marshal(payload)
	if w := cloudRequest(h, "POST", "/v1/onboarding/destination", string(body), session, true); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	watches, err := tenant.App.List(context.Background())
	if err != nil || len(watches) != 0 || sent != 1 {
		t.Fatal("setup created watch or resent", err, sent)
	}
}
