package watchcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestStatusOfflineDoesNotCreateState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	var out, errs bytes.Buffer
	if err := Execute("dev", []string{"status", "--state-dir", dir, "--json"}, &out, &errs); err != nil {
		t.Fatal(err, errs.String())
	}
	var result struct {
		Data localStatus `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Daemon != "unavailable" || result.Data.Installation != nil {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("read-only status created state")
	}
}

func TestStatusSeparatesReachabilityFromMonitoringHealth(t *testing.T) {
	dir := t.TempDir()
	creds, err := control.PrivateCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+creds.Admin {
			t.Error("unauthenticated probe")
		}
		_ = json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Data: watchrun.Doctor{Running: true, Healthy: false, LastError: "store pressure"}})
	}))
	defer server.Close()
	if err := control.SaveConnection(dir, server.URL); err != nil {
		t.Fatal(err)
	}
	s := readLocalStatus(context.Background(), dir, "dev")
	if s.Daemon != "ready" || s.Health == nil || s.Health.Healthy {
		t.Fatalf("%+v", s)
	}
}

func TestSetupDeclinedHasNoSideEffects(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	root := Root("dev")
	root.SetArgs([]string{"setup", "--state-dir", dir})
	root.SetIn(strings.NewReader("no\n"))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err == nil {
		t.Fatal("declined setup succeeded")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("declined setup created state")
	}
}
