package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/config"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/metrics"
	"github.com/ding-labs/ding/internal/server"
)

func TestIngestReportsStateLimit(t *testing.T) {
	e, err := evaluator.NewEngineWithLimits([]evaluator.EngineRule{{Name: "r", Condition: "value > 0", Cooldown: time.Hour}}, 100, evaluator.StateLimits{MaxLabelSets: 1, IdleTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Server: config.ServerConfig{Format: "json", MaxBodyBytes: 1 << 20}}
	s := server.New(e, nil, cfg, "", metrics.NewCollector(), nil, nil)
	for i, host := range []string{"a", "b"} {
		req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(`{"metric":"m","value":1,"host":"`+host+`"}`))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		want := 200
		if i == 1 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(w.Body.String(), "ding_state_rejections_total 1") {
		t.Fatal(w.Body.String())
	}
	s.Sweep(time.Now().Add(2 * time.Hour))
	if e.StateStats().LabelSets != 0 {
		t.Fatal("idle daemon state was not pruned")
	}
}
