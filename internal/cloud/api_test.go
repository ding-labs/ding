package cloud

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestHostedAPIBlocksLocalAdminAndCrossTenantObjects(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	for _, path := range []string{"/v1/backup", "/v1/console/backup", "/v1/browser/handoff", "/v1/ingest/health", "/v1/integrations/pair", "/v1/desktop/test"} {
		w := cloudRequest(h, "POST", path, "{}", sessions[0], true)
		if w.Code != 404 {
			t.Fatal("local route exposed", path, w.Code, w.Body.String())
		}
	}
	preview := cloudRequest(h, "POST", "/v1/onboarding/preview", `{"id":"health","url":"https://example.com/health","destination":"webhook","credential":"WEBHOOK"}`, sessions[0], true)
	var result struct {
		Data control.FirstWatchPreview `json:"data"`
	}
	if preview.Code != 200 || json.Unmarshal(preview.Body.Bytes(), &result) != nil {
		t.Fatal(preview.Code, preview.Body.String())
	}
	data, _ := json.Marshal(watchrun.ApplyRequest{Manifest: result.Data.Manifest})
	if w := cloudRequest(h, "POST", "/v1/apply", string(data), sessions[0], true); w.Code != 400 {
		t.Fatal("unreviewed apply accepted", w.Code)
	}
	data, _ = json.Marshal(watchrun.ApplyRequest{Manifest: result.Data.Manifest, Review: result.Data.Review.Review})
	if w := cloudRequest(h, "POST", "/v1/apply", string(data), sessions[0], true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := cloudRequest(h, "GET", "/v1/watches/health", "", sessions[1], false); w.Code != 404 {
		t.Fatal("cross-tenant watch exposed", w.Code, w.Body.String())
	}
	if w := cloudRequest(h, "POST", "/v1/apply", string(data), sessions[1], true); w.Code != 409 {
		t.Fatal("cross-tenant review accepted", w.Code, w.Body.String())
	}
	if w := cloudRequest(h, "POST", "/v1/cloud/secrets", `{"name":"TOKEN","value":"synthetic-secret"}`, sessions[0], true); w.Code != 200 || strings.Contains(w.Body.String(), "synthetic-secret") {
		t.Fatal("credential write failed or echoed value")
	}
	if w := cloudRequest(h, "GET", "/v1/cloud/secrets", "", sessions[1], false); w.Code != 200 || strings.Contains(w.Body.String(), "TOKEN") {
		t.Fatal("credential name crossed tenant")
	}
}
