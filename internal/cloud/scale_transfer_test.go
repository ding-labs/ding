package cloud

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaleRemoteLedgerRequiresAuthAndCannotOverwrite(t *testing.T) {
	c := scaleConfig{Name: "transfer", Rate: 1, Seconds: 1}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	out := t.TempDir()
	// Enable only the transfer route on this local TLS fixture.
	t.Setenv("DING_SCALE_PRIVATE_HOST", "ding-scale-app.railway.internal")
	scaleExternal(t, lab, out)
	raw, err := os.ReadFile(filepath.Join(out, "bootstrap.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bootstrap struct{ Admin string }
	if err := json.Unmarshal(raw, &bootstrap); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		token  string
		status int
	}{{"", 401}, {bootstrap.Admin, 204}, {bootstrap.Admin, 409}} {
		req, err := http.NewRequest("PUT", lab.http.URL+"/lab/ledger", strings.NewReader("private-ledger\n"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+check.token)
		response, err := lab.http.Client().Do(req)
		if err != nil {
			t.Fatal("transfer failed")
		}
		response.Body.Close()
		if response.StatusCode != check.status {
			t.Fatal(response.StatusCode)
		}
	}
	data, err := os.ReadFile(filepath.Join(out, "requests.jsonl"))
	if err != nil || string(data) != "private-ledger\n" {
		t.Fatal("journal changed", err)
	}
}
