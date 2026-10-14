package cloud

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPushBenchmarkNoRetryAccountsThrottle(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"code":"ingest_rate","message":"private failure text"}}`)
	}))
	defer s.Close()
	t.Setenv("DING_LOAD_TEST", "test")
	ledger := t.TempDir() + "/ledger"
	r, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{
		Producers: []PushProducer{{URL: s.URL, TokenEnv: "DING_LOAD_TEST", Body: []byte(`{}`)}},
		Duration:  time.Second, Rate: 20, Workers: 4, NoRetry: true, Ledger: ledger, Client: s.Client(), LedgerBatch: 8, CompactLedger: true,
	})
	if err != nil || r.Offered < 15 || r.Attempts != r.Offered || r.Statuses[429] != r.Offered || r.Throttled != r.Offered || r.Unconfirmed != 0 || r.FirstAccepted != 0 {
		t.Fatal(r, err)
	}
	if r.ErrorCodes["ingest_rate"] != r.Offered {
		t.Fatal(r)
	}
	raw, err := os.ReadFile(ledger)
	if err != nil || len(strings.Split(strings.TrimSpace(string(raw)), "\n")) != 1+r.Offered+r.Attempts || strings.Count(string(raw), `"body":{}`) != 1 {
		t.Fatal("compact ledger incomplete", err)
	}
	if strings.Contains(string(raw), "private failure text") {
		t.Fatal("recorded error text")
	}
}

func TestPushBenchmarkCountsMissedArrivalsAndQueueLatency(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(202)
		fmt.Fprint(w, `{"data":{"acceptanceId":"synthetic"}}`)
	}))
	defer s.Close()
	t.Setenv("DING_LOAD_TEST", "test")
	r, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{
		Producers: []PushProducer{{URL: s.URL, TokenEnv: "DING_LOAD_TEST", Body: []byte(`{}`)}},
		Duration:  time.Second, Rate: 40, Workers: 1, NoRetry: true, Ledger: t.TempDir() + "/ledger", Client: s.Client(),
	})
	if err != nil || r.Scheduled != 40 || r.Offered+r.GeneratorBusy != 40 || r.GeneratorBusy == 0 || r.Accepted != r.Offered || r.P99MS <= r.HTTPP99MS || r.ScheduleP99MS < 50 {
		t.Fatal(r, err)
	}
}
