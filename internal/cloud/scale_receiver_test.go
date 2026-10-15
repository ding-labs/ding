package cloud

import (
	"encoding/json"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

type scaleReceiver struct {
	server             *httptest.Server
	endpoint           string
	mu                 sync.Mutex
	accepted, attempts int
	recoverAt          time.Time
	ids                map[string]bool
	ledger             *os.File
}
type scaleDeliveryTransport struct {
	receiver *scaleReceiver
	next     http.RoundTripper
}

func (s scaleDeliveryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme == "https" && "https://"+r.URL.Host == s.receiver.endpoint {
		return s.receiver.server.Client().Transport.RoundTrip(r)
	}
	return s.next.RoundTrip(r)
}
func newScaleReceiver(t *testing.T, mode string, seconds int, path string) *scaleReceiver {
	f, err := mcpconfig.CreatePrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	s := &scaleReceiver{ids: map[string]bool{}, ledger: f}
	s.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == "slow" {
			time.Sleep(100 * time.Millisecond)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.attempts++
		if mode == "outage" && time.Now().Before(s.recoverAt) {
			w.WriteHeader(503)
			return
		}
		var event struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&event); err != nil || event.Data.ID == "" {
			w.WriteHeader(400)
			return
		}
		if err := json.NewEncoder(f).Encode(event); err != nil {
			w.WriteHeader(503)
			return
		}
		if err := f.Sync(); err != nil {
			w.WriteHeader(503)
			return
		}
		s.ids[event.Data.ID] = true
		s.accepted++
		w.WriteHeader(204)
	}))
	s.endpoint = s.server.URL
	t.Cleanup(s.server.Close)
	return s
}

type scaleReceiverState struct {
	Attempts, Accepted int
	IDs                map[string]bool
	Bytes              int64
}

func (s *scaleReceiver) setRecovery(t *testing.T, at time.Time) {
	t.Helper()
	s.mu.Lock()
	s.recoverAt = at
	s.mu.Unlock()
}
func (s *scaleReceiver) snapshot(t *testing.T) (scaleReceiverState, []byte) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.ledger.Name())
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for id := range s.ids {
		ids[id] = true
	}
	return scaleReceiverState{s.attempts, s.accepted, ids, int64(len(raw))}, raw
}
