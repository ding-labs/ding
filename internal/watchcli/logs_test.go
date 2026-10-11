package watchcli

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestLogCLIChild(t *testing.T) {
	if os.Getenv("DING_LOG_CLI_CHILD") != "1" {
		return
	}
	for _, level := range []string{"WARN", "INFO"} {
		for range 40 {
			fmt.Fprintln(os.Stdout, level+" message")
			time.Sleep(50 * time.Millisecond)
		}
	}
	os.Exit(7)
}

func TestLogsExecThroughRealIngestion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("capture is not yet qualified on Windows")
	}
	dir := t.TempDir()
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := watchrun.New(s)
	_, err = a.Apply(context.Background(), watchrun.ApplyRequest{Manifest: `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: app-logs}
spec:
  source: {type: push}
  condition: {field: warning_match_ratio, operator: gt, value: 0.5}
`})
	if err != nil {
		t.Fatal(err)
	}
	credentials, _ := control.PrivateCredentials(dir)
	server := httptest.NewServer(control.Handler(a, credentials))
	defer server.Close()
	control.SaveConnection(dir, server.URL)
	t.Setenv("DING_LOG_CLI_CHILD", "1")
	var out, diagnostics bytes.Buffer
	err = Execute("test", []string{"logs", "exec", "--state-dir", dir, "--watch", "app-logs", "--window", "1s", "--min-events", "1", "--", os.Args[0], "-test.run=^TestLogCLIChild$"}, &out, &diagnostics)
	if ExitCode(err) != 7 {
		t.Fatal(err, diagnostics.String())
	}
	if out.String() != strings.Repeat("WARN message\n", 40)+strings.Repeat("INFO message\n", 40) {
		t.Fatal("application stdout changed")
	}
	kinds := map[string]int{}
	err = s.View(context.Background(), func(tx *store.Tx) error {
		events, err := tx.Events("app-logs", 0, 100)
		for _, event := range events {
			kinds[event.Type]++
		}
		return err
	})
	if err != nil || kinds["firing"] != 1 || kinds["recovered"] != 1 {
		t.Fatal(err, kinds, diagnostics.String())
	}
}
