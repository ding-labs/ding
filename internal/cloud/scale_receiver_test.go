package cloud

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type scaleReceiver struct {
	server             *httptest.Server
	mu                 sync.Mutex
	accepted, attempts int
	recoverAt          time.Time
}
type scaleDeliveryTransport struct {
	receiver *scaleReceiver
	next     http.RoundTripper
}

func (s scaleDeliveryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == s.receiver.server.Listener.Addr().String() {
		return s.receiver.server.Client().Transport.RoundTrip(r)
	}
	return s.next.RoundTrip(r)
}
func newScaleReceiver(t *testing.T, mode string, seconds int) *scaleReceiver {
	s := &scaleReceiver{recoverAt: time.Now().Add(time.Duration(seconds/2) * time.Second)}
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
		s.accepted++
		w.WriteHeader(204)
	}))
	t.Cleanup(s.server.Close)
	return s
}
