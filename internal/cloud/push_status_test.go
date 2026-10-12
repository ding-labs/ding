package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestPushScopeUpdatesDiagnosticsAndPreviewIsolation(t *testing.T) {
	s, h, sessions, grants := pushSetup(t)
	body := `{"repo":"private-repo-value","failed":true}`
	first := pushRequest(h, grants[0].Token, "private-key", body)
	receiptData(t, first)
	if first.Header().Get("X-Ding-Request-ID") == "" {
		t.Fatal("missing diagnostic ID")
	}
	pushRequest(h, grants[0].Token, "private-key", strings.Replace(body, "true", "false", 1))
	status := cloudRequest(h, "GET", "/v1/ingest-status/ci", "", sessions[0], false)
	if status.Code != 200 || !strings.Contains(status.Body.String(), "idempotency_conflict") {
		t.Fatal(status.Code, status.Body.String())
	}
	for _, secret := range []string{grants[0].Token, "private-key", "private-repo-value"} {
		if strings.Contains(status.Body.String(), secret) {
			t.Fatal("diagnostic disclosed input")
		}
	}
	foreign := cloudRequest(h, "GET", "/v1/ingest-status/ci", "", sessions[1], false)
	if foreign.Code != 200 || !strings.Contains(foreign.Body.String(), `"lastAcceptance":null`) || !strings.Contains(foreign.Body.String(), `"recent":[]`) {
		t.Fatal("cross-workspace diagnostics", foreign.Body.String())
	}
	keys, err := s.DB.ServiceKeys(context.Background(), sessions[0].Workspace, grants[0].ID)
	if err != nil || len(keys) != 1 || keys[0].LastUsedAt == nil {
		t.Fatal("missing coalesced key usage", keys, err)
	}
	fixture, _ := json.Marshal(map[string]any{"manifest": pushFixture, "watch": "ci", "fixture": body})
	preview := cloudRequest(h, "POST", "/v1/tools/project", string(fixture), sessions[0], true)
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), `"persisted":false`) {
		t.Fatal(preview.Code, preview.Body.String())
	}
	tenant, _ := s.Pool.Get(context.Background(), sessions[0].Workspace)
	if _, err := tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: strings.Replace(pushFixture, "id: ci", "id: other", 1)}); err != nil {
		t.Fatal(err)
	}
	update := cloudRequest(h, "PATCH", "/v1/services/"+grants[0].ID, `{"name":"CI","scopes":["ingest"],"watches":["other"]}`, sessions[0], true)
	if update.Code != 200 {
		t.Fatal(update.Code, update.Body.String())
	}
	if w := pushRequest(h, grants[0].Token, "private-key", body); w.Code != 403 {
		t.Fatal("removed scope accepted replay", w.Code)
	}
	denied := cloudRequest(h, "PATCH", "/v1/services/"+grants[0].ID, `{"name":"CI","scopes":["ingest"],"watches":["ci"]}`, sessions[1], true)
	if denied.Code == 200 {
		t.Fatal("other workspace changed producer")
	}
	if err := tenant.App.Store.View(context.Background(), func(tx *store.Tx) error {
		u, e := tx.PushUsage()
		if u.Requests != 1 {
			t.Error("preview or rejection counted", u)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	metrics := httptest.NewRecorder()
	s.Pool.Metrics(metrics, httptest.NewRequest("GET", "http://localhost/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `reason="digest_conflict"} 1`) {
		t.Fatal(metrics.Body.String())
	}
}

func TestEighteenPushResourcesAndIndependentGrouping(t *testing.T) {
	s, h, sessions, grants := pushSetup(t)
	tenant, _ := s.Pool.Get(context.Background(), sessions[0].Workspace)
	for i := 0; i < 18; i++ {
		if _, err := tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: strings.Replace(pushFixture, "id: ci", fmt.Sprintf("id: ci-%d", i), 1)}); err != nil {
			t.Fatal(i, err)
		}
	}
	for i := 0; i < 18; i++ {
		receiptData(t, pushRequest(h, grants[0].Token, fmt.Sprintf("fail-%d", i), fmt.Sprintf(`{"repo":"repo-%d","failed":true}`, i)))
	}
	receiptData(t, pushRequest(h, grants[0].Token, "recover-one", `{"repo":"repo-0","failed":false}`))
	inspection, err := tenant.App.Inspect(context.Background(), "ci")
	if err != nil || len(inspection.Entities) != 18 {
		t.Fatal(len(inspection.Entities), err)
	}
	open := 0
	for _, body := range inspection.Entities {
		var state struct {
			Open bool `json:"open"`
		}
		if json.Unmarshal(body, &state) != nil {
			t.Fatal("bad condition state")
		}
		if state.Open {
			open++
		}
	}
	if open != 17 {
		t.Fatal("one repository changed another incident", open)
	}
}
