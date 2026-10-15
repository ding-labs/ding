package cloud

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

type scaleReceiverBootstrap struct {
	Version                int
	URL, CA, Token, Ledger string
}

func loadScaleReceiver(t *testing.T, path string) *scaleReceiver {
	t.Helper()
	var b scaleReceiverBootstrap
	if err := mcpconfig.ReadPrivateJSON(path, &b, true); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(b.URL)
	_, tokenErr := hex.DecodeString(b.Token)
	if err != nil || u.Scheme != "https" || !net.ParseIP(u.Hostname()).IsLoopback() || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Path != "" || b.Version != 1 || len(b.Token) != 64 || tokenErr != nil {
		t.Fatal("invalid local receiver bootstrap")
	}
	if filepath.Clean(b.Ledger) != filepath.Join(filepath.Dir(path), "receiver.jsonl") {
		t.Fatal("receiver ledger must share its private bootstrap directory")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(b.CA)) {
		t.Fatal("invalid receiver fixture CA")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	t.Cleanup(transport.CloseIdleConnections)
	return &scaleReceiver{endpoint: b.URL, remoteToken: b.Token, remoteLedger: b.Ledger, client: &http.Client{Transport: transport, Timeout: 5 * time.Second}}
}

func (s *scaleReceiver) remoteCall(t *testing.T, method, path string, input, output any) {
	t.Helper()
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	r, err := http.NewRequest(method, s.endpoint+path, body)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+s.remoteToken)
	response, err := s.client.Do(r)
	if err != nil {
		t.Fatal("local receiver control unavailable", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatal("local receiver control status", response.StatusCode)
	}
	if output != nil {
		if err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(output); err != nil {
			t.Fatal("invalid receiver snapshot", err)
		}
	}
}

func (s *scaleReceiver) remoteSnapshot(t *testing.T) (scaleReceiverState, []byte) {
	t.Helper()
	var state scaleReceiverState
	s.remoteCall(t, "GET", "/lab/state", nil, &state)
	if state.Bytes < 0 || state.Bytes > 64<<20 {
		t.Fatal("receiver evidence exceeds read budget")
	}
	file, err := mcpconfig.OpenPrivate(s.remoteLedger)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, state.Bytes))
	if err != nil || int64(len(raw)) != state.Bytes {
		t.Fatal("incomplete receiver snapshot", err)
	}
	return state, raw
}

// TestLocalScaleReceiver is an opt-in helper process, never a production route.
// The launcher supplies a distinct cgroup when using a Linux container.
func TestLocalScaleReceiver(t *testing.T) {
	if os.Getenv("DING_SCALE_RECEIVER_ONLY") != "1" {
		t.Skip("local receiver helper process")
	}
	out := os.Getenv("DING_SCALE_RECEIVER_OUTPUT")
	if !filepath.IsAbs(out) {
		t.Fatal("private absolute output directory required")
	}
	mode := os.Getenv("DING_SCALE_RECEIVER_MODE")
	if mode != "fast" && mode != "slow" && mode != "outage" {
		t.Fatal("invalid receiver fixture mode")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	receiver := newScaleReceiver(t, mode, 0, filepath.Join(out, "receiver.jsonl"))
	bootstrap := scaleReceiverBootstrap{1, receiver.endpoint, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: receiver.server.Certificate().Raw})), receiver.admin, receiver.ledger.Name()}
	file, err := mcpconfig.CreatePrivate(filepath.Join(out, ".bootstrap.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewEncoder(file).Encode(bootstrap)
	if err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closed != nil {
		t.Fatal(closed)
	}
	if err = os.Rename(filepath.Join(out, ".bootstrap.tmp"), filepath.Join(out, "bootstrap.json")); err != nil {
		t.Fatal(err)
	}
	// The owned launcher terminates this process after exporting evidence.
	select {}
}

func TestScaleReceiverProcessWithDurableEvidence(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	log, err := mcpconfig.CreatePrivate(filepath.Join(out, "process.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	command := exec.Command(executable, "-test.run=^TestLocalScaleReceiver$", "-test.timeout=20s")
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "DING_SCALE_RECEIVER_") {
			command.Env = append(command.Env, env)
		}
	}
	command.Env = append(command.Env, "DING_SCALE_RECEIVER_ONLY=1", "DING_SCALE_RECEIVER_OUTPUT="+out, "DING_SCALE_RECEIVER_MODE=outage", "DING_SCALE_RECEIVER_LISTEN=127.0.0.1:0")
	command.Stdout, command.Stderr = log, log
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	bootstrap := filepath.Join(out, "bootstrap.json")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err = os.Stat(bootstrap); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("receiver process did not bootstrap")
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Setenv("DING_SCALE_RECEIVER_FILE", bootstrap)
	c := scaleConfig{Name: "receiver-process", Rate: 10, Seconds: 1, Watches: 4, DeliveryWorkers: 4, Condition: "alerts", Receiver: "outage"}
	if err = c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	lab.receiver.setRecovery(t, time.Now().Add(2*time.Second))
	result, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Duration: time.Second, Rate: 10, Ledger: t.TempDir() + "/ledger", LedgerBatch: 8, CompactLedger: true, NoRetry: true})
	if err != nil || result.Accepted != 10 {
		t.Fatal(result, err)
	}
	deadline = time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		pending := 0
		for _, a := range scaleAccounts(t, lab) {
			pending += a.Rows.Pending
		}
		if pending == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	state, problems := scaleReceiverEvidence(t, lab, t.TempDir())
	if len(problems) > 0 || len(state.IDs) != 10 || state.Attempts <= state.Accepted {
		t.Fatal("receiver process did not recover with durable IDs", problems, len(state.IDs), state.Attempts, state.Accepted)
	}
}
