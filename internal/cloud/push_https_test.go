package cloud

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPushOverHTTPSBodyDeadlineAndWorkspaceFairness(t *testing.T) {
	s, h, _, grants := pushSetup(t)
	s.Push.WorkspaceConcurrent = 1
	s.Push.ReadSeconds = 1
	server := httptest.NewUnstartedServer(h)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	reader, writer := io.Pipe()
	defer writer.Close()
	req, _ := http.NewRequest("POST", server.URL+"/v1/ingest/ci", reader)
	req.Host = "ding.example"
	req.Header.Set("Authorization", "Bearer "+grants[0].Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "slow-network")
	done := make(chan error, 1)
	go func() {
		response, err := server.Client().Do(req)
		if err == nil {
			defer response.Body.Close()
			if response.StatusCode != 400 {
				err = fmt.Errorf("slow body response %d", response.StatusCode)
			}
		}
		done <- err
	}()
	// A body byte proves the request reached the transport, then remains stalled.
	if _, err := writer.Write([]byte("{")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.pushMu.Lock()
		b := s.pushBudgets["workspace:"+grants[0].Workspace]
		active := b != nil && b.active > 0
		s.pushMu.Unlock()
		if active {
			break
		}
		time.Sleep(time.Millisecond)
	}
	send := func(token string) *http.Response {
		r, _ := http.NewRequest("POST", server.URL+"/v1/ingest/ci", bytes.NewBufferString(`{"repo":"other","failed":false}`))
		r.Host = "ding.example"
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "normal")
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := send(grants[0].Token)
	response.Body.Close()
	if response.StatusCode != 429 {
		t.Fatal("busy workspace unbounded", response.StatusCode)
	}
	response = send(grants[1].Token)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatal("other workspace blocked", response.StatusCode, string(body))
	}
	select {
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "stream error") {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("real body read deadline failed")
	}
	health, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL+"/healthz", nil)
	health.Host = "ding.example"
	response, err := server.Client().Do(health)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("health unavailable", response.StatusCode)
	}
}

func TestPushBenchmarkWritesReconciliableLedgerThroughHTTPS(t *testing.T) {
	_, h, _, grants := pushSetup(t)
	// The test proxy preserves the production Host check; TLS verification uses
	// the test server CA, never InsecureSkipVerify.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "ding.example"; h.ServeHTTP(w, r) }))
	defer server.Close()
	t.Setenv("DING_BENCH_TEST_PRODUCER", grants[0].Token)
	ledger := t.TempDir() + "/requests.jsonl"
	result, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Duration: time.Second, Rate: 10, Ledger: ledger, Client: server.Client(), Producers: []PushProducer{{URL: server.URL + "/v1/ingest/ci", TokenEnv: "DING_BENCH_TEST_PRODUCER", Body: []byte(`{"repo":"benchmark","failed":false}`)}}})
	if err != nil || result.Accepted < 8 || result.Offered != result.Accepted || result.Unconfirmed != 0 || result.GeneratorBusy != 0 {
		t.Fatal(result, err)
	}
	raw, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), grants[0].Token) {
		t.Fatal("ledger leaked producer credential")
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != result.Offered+result.Attempts {
		t.Fatal("incomplete ledger", len(lines), result)
	}
}

func TestHundredWorkspacePushOverHTTPS(t *testing.T) {
	s, h, _, grants := pushSetup(t)
	ctx := context.Background()
	for i := len(grants); i < MaxAccounts; i++ {
		enrolled, err := s.DB.Enroll(ctx, "qualification", fmt.Sprintf("customer-%d", i), MaxAccounts)
		if err != nil {
			t.Fatal(err)
		}
		tenant, err := s.Pool.Get(ctx, enrolled.Workspace.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tenant.App.Apply(ctx, watchrun.ApplyRequest{Manifest: pushFixture}); err != nil {
			t.Fatal(err)
		}
		principal, err := s.DB.Principal(ctx, enrolled.User, enrolled.Workspace.ID, "browser", nil)
		if err != nil {
			t.Fatal(err)
		}
		lifetimes, err := s.producerLifetimes(ctx, enrolled.Workspace.ID, []string{"ci"})
		if err != nil {
			t.Fatal(err)
		}
		grant, err := s.DB.CreateBoundService(ctx, principal, "load", []string{"ingest"}, []string{"ci"}, 1, lifetimes)
		if err != nil {
			t.Fatal(err)
		}
		grants = append(grants, grant)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "ding.example"; h.ServeHTTP(w, r) }))
	defer server.Close()
	producers := []PushProducer{}
	for i, grant := range grants {
		env := fmt.Sprintf("DING_QUALIFICATION_TOKEN_%d", i)
		t.Setenv(env, grant.Token)
		producers = append(producers, PushProducer{URL: server.URL + "/v1/ingest/ci", TokenEnv: env, Body: []byte(`{"repo":"same-name","failed":false}`)})
	}
	// Tenant coverage is deterministic; a timed generator may offer fewer than
	// 100 inputs on a busy race-test host. Throughput is evidence, not a gate.
	for i, grant := range grants {
		req, err := http.NewRequestWithContext(ctx, "POST", producers[i].URL, bytes.NewReader(producers[i].Body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+grant.Token)
		req.Header.Set("Idempotency-Key", "workspace-coverage")
		req.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusAccepted {
			t.Fatalf("workspace %d: status=%d body=%s err=%v", i, response.StatusCode, raw, err)
		}
	}
	result, err := BenchmarkPush(ctx, PushBenchmarkOptions{Duration: 2 * time.Second, Rate: 100, Ledger: t.TempDir() + "/ledger.jsonl", Client: server.Client(), Producers: producers})
	if err != nil || result.Accepted < 1 || result.Accepted != result.Offered || result.Unconfirmed != 0 || result.Rejected != 0 {
		t.Fatal(result, err)
	}
	var accounted int64
	for _, grant := range grants {
		tenant, _ := s.Pool.Get(ctx, grant.Workspace)
		if err := tenant.App.Store.View(ctx, func(tx *store.Tx) error {
			u, e := tx.PushUsage()
			if u.Requests < 1 {
				return fmt.Errorf("workspace received no input")
			}
			accounted += u.Requests
			return e
		}); err != nil {
			t.Fatal(err)
		}
	}
	if accounted != int64(result.Accepted+len(grants)) {
		t.Fatal("ledger differs from workspace counters", accounted, result)
	}
	t.Logf("100 isolated workspace stores through HTTPS: %+v", result)
}
