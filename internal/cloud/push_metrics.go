package cloud

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	state "github.com/ding-labs/ding/internal/serverstate"
)

type pushMetrics struct {
	parse, commit pushTiming
	reasons       [7]atomic.Int64
	status        [6]atomic.Int64
	duplicates    atomic.Int64
	active        atomic.Int64
	seconds       atomic.Int64
	buckets       [5]atomic.Int64
}
type pushResponse struct {
	http.ResponseWriter
	status                           int
	workspace, watch, lifetime, code string
}

func (w *pushResponse) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *pushResponse) Write(body []byte) (int, error) {
	if w.status >= 400 && len(body) <= 4096 {
		var out struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &out) == nil {
			w.code = out.Error.Code
		}
	}
	return w.ResponseWriter.Write(body)
}
func (w *pushResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (s *Server) measuredPush(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := state.ID()[:24]
	w.Header().Set("X-Ding-Request-ID", requestID)
	out := &pushResponse{ResponseWriter: w, status: 200}
	s.Pool.ingest.active.Add(1)
	defer func() {
		s.Pool.ingest.active.Add(-1)
		s.Pool.ingest.status[out.status/100].Add(1)
		reason := -1
		switch {
		case out.status == 401 || out.status == 403:
			reason = 0
		case out.status == 400 || out.status == 413 || out.status == 415:
			reason = 1
		case out.code == "idempotency_conflict":
			reason = 2
		case out.status == 429:
			reason = 3
		case out.code == "acceptance_uncertain":
			reason = 4
		case out.status >= 500:
			reason = 5
		case out.status == 409:
			reason = 6
		}
		if reason >= 0 {
			s.Pool.ingest.reasons[reason].Add(1)
		}
		if out.workspace != "" {
			s.recordPushOutcome(out.workspace, pushOutcome{RequestID: requestID, Watch: out.watch, Lifetime: out.lifetime, At: start.UTC(), Status: out.status, Code: out.code})
		}
		elapsed := time.Since(start)
		s.Pool.ingest.seconds.Add(elapsed.Nanoseconds())
		for i, bound := range []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 10 * time.Second} {
			if elapsed <= bound {
				s.Pool.ingest.buckets[i].Add(1)
			}
		}
	}()
	s.servePush(out, r)
}
func (m *pushMetrics) write(w http.ResponseWriter) {
	for i, label := range []string{"authorization", "invalid_input", "digest_conflict", "throttle", "commit_uncertain", "unavailable", "lifecycle"} {
		fmt.Fprintf(w, "ding_cloud_ingest_rejections_total{reason=%q} %d\n", label, m.reasons[i].Load())
	}
	m.parse.write(w, "parse")
	m.commit.write(w, "commit")
	total := int64(0)
	for status := 1; status < 6; status++ {
		n := m.status[status].Load()
		total += n
		fmt.Fprintf(w, "ding_cloud_ingest_responses_total{class=\"%dxx\"} %d\n", status, n)
	}
	fmt.Fprintf(w, "ding_cloud_ingest_replays_total %d\nding_cloud_ingest_active %d\n", m.duplicates.Load(), m.active.Load())
	for i, bound := range []string{"0.1", "0.5", "1", "2", "10"} {
		fmt.Fprintf(w, "ding_cloud_ingest_seconds_bucket{le=\"%s\"} %d\n", bound, m.buckets[i].Load())
	}
	fmt.Fprintf(w, "ding_cloud_ingest_seconds_bucket{le=\"+Inf\"} %d\nding_cloud_ingest_seconds_count %d\nding_cloud_ingest_seconds_sum %g\n", total, total, float64(m.seconds.Load())/1e9)
}

type pushTiming struct {
	count, sum atomic.Int64
	buckets    [5]atomic.Int64
}

func (m *pushTiming) observe(elapsed time.Duration) {
	m.count.Add(1)
	m.sum.Add(elapsed.Nanoseconds())
	for i, limit := range []time.Duration{time.Millisecond, 10 * time.Millisecond, 100 * time.Millisecond, time.Second, 10 * time.Second} {
		if elapsed <= limit {
			m.buckets[i].Add(1)
		}
	}
}
func (m *pushTiming) write(w http.ResponseWriter, phase string) {
	for i, limit := range []string{"0.001", "0.01", "0.1", "1", "10"} {
		fmt.Fprintf(w, "ding_cloud_ingest_phase_seconds_bucket{phase=%q,le=%q} %d\n", phase, limit, m.buckets[i].Load())
	}
	fmt.Fprintf(w, "ding_cloud_ingest_phase_seconds_bucket{phase=%q,le=\"+Inf\"} %d\nding_cloud_ingest_phase_seconds_count{phase=%q} %d\nding_cloud_ingest_phase_seconds_sum{phase=%q} %g\n", phase, m.count.Load(), phase, m.count.Load(), phase, float64(m.sum.Load())/1e9)
}
