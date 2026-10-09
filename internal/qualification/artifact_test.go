// Package qualification exercises the executable that users actually install.
package qualification

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/replay"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func TestNativeArtifact(t *testing.T) {
	binary := os.Getenv("DING_BINARY")
	if binary == "" {
		t.Skip("set DING_BINARY to the freshly built native release executable")
	}
	binary, err := filepath.Abs(binary)
	must(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cli := func(dir string, args ...string) json.RawMessage {
		t.Helper()
		args = append(args, "--state-dir", dir, "--json")
		out, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", args, err, out)
		}
		var envelope struct {
			APIVersion string          `json:"apiVersion"`
			Data       json.RawMessage `json:"data"`
		}
		must(t, json.Unmarshal(out, &envelope))
		if envelope.APIVersion != watch.APIVersion {
			t.Fatal("invalid API envelope", string(out))
		}
		return envelope.Data
	}
	dir := t.TempDir()
	var version map[string]string
	must(t, json.Unmarshal(cli(dir, "version"), &version))
	if version["os"] != runtime.GOOS || version["arch"] != runtime.GOARCH {
		t.Fatal("artifact is not native", version, runtime.GOOS, runtime.GOARCH)
	}
	info, err := os.Stat(binary)
	must(t, err)
	t.Logf("native artifact %s/%s: %d executable bytes; version %s", runtime.GOOS, runtime.GOARCH, info.Size(), version["version"])
	start := func(stateDir string) func() {
		t.Helper()
		log, err := os.CreateTemp(t.TempDir(), "daemon-*.log")
		must(t, err)
		cmd := exec.CommandContext(ctx, binary, "daemon", "--state-dir", stateDir, "--listen", "127.0.0.1:0")
		cmd.Stdout, cmd.Stderr = log, log
		must(t, cmd.Start())
		stopped := false
		stop := func() {
			if !stopped {
				stopped = true
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				_ = log.Close()
			}
		}
		t.Cleanup(stop)
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			client, err := control.Connect(stateDir)
			if err == nil {
				_, err = client.Call(ctx, "GET", "/v1/doctor", nil)
				if err == nil {
					return stop
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		stop()
		data, _ := os.ReadFile(log.Name())
		t.Fatalf("daemon did not start: %s", data)
		return stop
	}
	stop := start(dir)
	if os.Getenv("DING_EXPECT_CONSOLE") == "1" {
		client, err := control.Connect(dir)
		must(t, err)
		read := func(path string) []byte {
			t.Helper()
			req, err := http.NewRequestWithContext(ctx, "GET", client.URL+path, nil)
			must(t, err)
			res, err := http.DefaultClient.Do(req)
			must(t, err)
			defer res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatalf("console artifact %s: %d", path, res.StatusCode)
			}
			data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
			must(t, err)
			return data
		}
		page := read("/ui/watches/artifact")
		assets := regexp.MustCompile(`(?:src|href)="(/ui/assets/[^" ]+)"`).FindAllSubmatch(page, -1)
		if len(assets) < 2 {
			t.Fatal("embedded console did not reference CSS and JavaScript")
		}
		for _, asset := range assets {
			if len(read(string(asset[1]))) < 100 {
				t.Fatal("empty embedded asset")
			}
		}
	}

	manifest := filepath.Join(t.TempDir(), "watch.yaml")
	must(t, os.WriteFile(manifest, []byte(`apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: artifact}
spec:
  source: {type: push, fields: {value: value}}
  condition: {field: value, operator: gte, value: 1}
`), 0600))
	cli(dir, "validate", manifest)
	cli(dir, "apply", manifest, "--dry-run")
	cli(dir, "apply", manifest)
	client, err := control.Connect(dir)
	must(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "tokens.json"))
	must(t, err)
	var credentials control.Credentials
	must(t, json.Unmarshal(data, &credentials))
	client.Token = credentials.Ingest
	_, err = client.Call(ctx, "POST", "/v1/ingest/artifact", map[string]int{"value": 2})
	must(t, err)
	var page store.EventPage
	must(t, json.Unmarshal(cli(dir, "events", "--watch", "artifact"), &page))
	var eventID string
	for _, event := range page.Events {
		if event.Type == "firing" {
			eventID = event.ID
		}
	}
	if eventID == "" {
		t.Fatal("accepted push has no committed firing", string(cli(dir, "events")))
	}
	proofFile := filepath.Join(t.TempDir(), "evidence.json")
	proofRaw := cli(dir, "events", "inspect", eventID)
	var proof replay.Evidence
	must(t, json.Unmarshal(proofRaw, &proof))
	must(t, replay.Verify(proof))
	must(t, os.WriteFile(proofFile, proofRaw, 0600))
	cli(dir, "replay", proofFile)
	// Exercise the exact manifest shipped beside the README quickstart.
	quickstart, err := filepath.Abs(filepath.Join("..", "..", "ding.yaml.example"))
	must(t, err)
	cli(dir, "validate", quickstart)
	cli(dir, "apply", quickstart, "--dry-run")
	cli(dir, "apply", quickstart)
	for _, value := range []int{350, 350, 100} {
		_, err = client.Call(ctx, "POST", "/v1/ingest/latency", map[string]int{"latency_ms": value})
		must(t, err)
	}
	var quickstartEvents store.EventPage
	must(t, json.Unmarshal(cli(dir, "events", "--watch", "latency"), &quickstartEvents))
	counts := map[string]int{}
	for _, event := range quickstartEvents.Events {
		counts[event.Type]++
	}
	if counts["firing"] != 1 || counts["recovered"] != 1 {
		t.Fatal("quickstart transition/recovery", counts)
	}
	backup := filepath.Join(t.TempDir(), "verified.db")
	cli(dir, "backup", "--out", backup)
	cli(dir, "export", "--watch", "artifact")
	stop()
	stop = start(dir)
	if !strings.Contains(string(cli(dir, "events", "--watch", "artifact")), eventID) {
		t.Fatal("crash restart lost committed event")
	}
	stop()
	restored := t.TempDir()
	in, err := os.Open(backup)
	must(t, err)
	defer in.Close()
	out, err := os.OpenFile(filepath.Join(restored, "ding.db"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	must(t, err)
	_, err = io.Copy(out, in)
	must(t, err)
	must(t, out.Close())
	stop = start(restored)
	defer stop()
	if !strings.Contains(string(cli(restored, "events", "--watch", "artifact")), eventID) {
		t.Fatal("backup restore lost committed event")
	}
	var doctor struct {
		Healthy bool `json:"healthy"`
	}
	must(t, json.Unmarshal(cli(restored, "doctor"), &doctor))
	if !doctor.Healthy {
		t.Fatal("restored daemon is unhealthy")
	}
	t.Log("fresh install, apply, authenticated push, event replay, force-kill restart, and offline backup restore passed")
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(fmt.Errorf("qualification: %w", err))
	}
}
