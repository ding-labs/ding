package cloud

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPushBenchmarkNoRetryAccountsThrottle(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer s.Close()
	t.Setenv("DING_LOAD_TEST", "test")
	r, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{
		Producers: []PushProducer{{URL: s.URL, TokenEnv: "DING_LOAD_TEST", Body: []byte(`{}`)}},
		Duration:  time.Second, Rate: 20, Workers: 4, NoRetry: true, Ledger: t.TempDir() + "/ledger", Client: s.Client(),
	})
	if err != nil || r.Offered < 15 || r.Attempts != r.Offered || r.Statuses[429] != r.Offered || r.Unconfirmed != r.Offered || r.FirstAccepted != 0 {
		t.Fatal(r, err)
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
