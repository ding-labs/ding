package watchcli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestRuntimeCLI(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	creds, err := control.PrivateCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(control.Handler(watchrun.New(s), creds))
	defer server.Close()
	if err := control.SaveConnection(dir, server.URL); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"apply", "../../examples/watches/api-health.yaml", "--dry-run"}, {"apply", "../../examples/watches/api-health.yaml"}, {"watch", "list"}, {"watch", "inspect", "api-health"}, {"watch", "list", "--json"}, {"watch", "pause", "api-health", "--json"}, {"watch", "resume", "api-health", "--json"}, {"watch", "delete", "api-health", "--cancel-pending", "--json"}} {
		var out, errs bytes.Buffer
		args = append(args, "--state-dir", dir)
		if err := Execute("test", args, &out, &errs); err != nil {
			t.Fatal(args, err, errs.String())
		}
		if out.Len() == 0 {
			t.Fatal("empty output")
		}
	}
	for _, args := range [][]string{{"apply", "missing"}, {"apply", "../../examples/watches/api-health.yaml", "--expected-revision", "bad"}, {"apply", "../../examples/watches/api-health.yaml", "--watch", "api-health", "--expected-revision", "bad"}, {"watch", "inspect", "missing"}} {
		var out, errs bytes.Buffer
		args = append(args, "--state-dir", dir, "--json")
		if err := Execute("test", args, &out, &errs); err == nil {
			t.Fatal("accepted", args)
		}
		if errs.Len() == 0 || out.Len() != 0 {
			t.Fatal(out.String(), errs.String())
		}
	}
	var out bytes.Buffer
	if err := Execute("test", []string{"watch", "list", "--state-dir", t.TempDir(), "--json"}, &out, &out); err == nil {
		t.Fatal("connected to missing daemon")
	}
}
func TestDaemonLifecycle(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := Root("test")
	root.SetContext(ctx)
	root.SetArgs([]string{"daemon", "--state-dir", dir, "--listen", "127.0.0.1:0"})
	var logs bytes.Buffer
	root.SetErr(&logs)
	root.SetOut(&logs)
	done := make(chan error, 1)
	go func() { done <- root.Execute() }()
	deadline := time.Now().Add(5 * time.Second)
	var client control.Client
	var err error
	for time.Now().Before(deadline) {
		client, err = control.Connect(dir)
		if err == nil {
			if _, err = client.Call(context.Background(), "GET", "/health", nil); err == nil {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		cancel()
		<-done
		t.Fatal(err, logs.String())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err, logs.String())
		}
	case <-time.After(8 * time.Second):
		t.Fatal("daemon did not stop")
	}
	if _, err := os.Stat(filepath.Join(dir, "ding.db")); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal("writer lock not released", err)
	}
	s.Close()
}
