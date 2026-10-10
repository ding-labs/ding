package watchcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/watch"
)

func TestUIWatchUsesFreshHandoffWithoutExposingAdminCredential(t *testing.T) {
	dir := t.TempDir()
	credentials, err := control.PrivateCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	var endpoint string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+credentials.Admin {
			t.Error("missing local authority")
		}
		var data any = map[string]bool{"exists": true}
		if r.URL.Path == "/v1/watches/api-health" {
			seen = true
		} else if r.URL.Path == "/v1/browser/handoff" {
			if !seen {
				t.Error("minted handoff before checking watch")
			}
			data = control.BrowserHandoff{URL: endpoint + "/ui/#handoff=one-use-token"}
		} else {
			t.Error("unexpected path", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Data: data})
	}))
	defer server.Close()
	endpoint = server.URL
	if err := control.SaveConnection(dir, endpoint); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Execute("dev", []string{"ui", "--watch", "api-health", "--no-open", "--state-dir", dir}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/ui/watches/api-health#handoff=one-use-token") || strings.Contains(out.String(), credentials.Admin) {
		t.Fatal("incorrect handoff boundary")
	}
}
