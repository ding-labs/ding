package cloud

import (
	"crypto/subtle"
	"encoding/json"
	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type scaleReceiver struct {
	remoteToken, remoteLedger string
	client                    *http.Client
	admin                     string
	server                    *httptest.Server
	endpoint                  string
	mu                        sync.Mutex
	accepted, attempts        int
	recoverAt                 time.Time
	ids                       map[string]bool
	ledger                    *os.File
}
type scaleDeliveryTransport struct {
	receiver *scaleReceiver
	next     http.RoundTripper
}

type scaleReceiverRecord struct {
	Workspace string `json:"workspace"`
	Data      struct {
		ID string `json:"id"`
	} `json:"data"`
}

func scaleReceiverKey(workspace, event string) string { return workspace + ":" + event }

func (s scaleDeliveryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme == "https" && "https://"+r.URL.Host == s.receiver.endpoint {
		return s.receiver.client.Transport.RoundTrip(r)
	}
	return s.next.RoundTrip(r)
}
func newScaleReceiver(t *testing.T, mode string, seconds int, path string) *scaleReceiver {
	if file := os.Getenv("DING_SCALE_RECEIVER_FILE"); file != "" {
		return loadScaleReceiver(t, file)
	}
	f, err := mcpconfig.CreatePrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	s := &scaleReceiver{ids: map[string]bool{}, ledger: f, admin: state.ID()}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/lab/") {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.admin)) != 1 {
				w.WriteHeader(401)
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			switch r.URL.Path {
			case "/lab/state":
				info, err := f.Stat()
				if err != nil {
					w.WriteHeader(500)
					return
				}
				json.NewEncoder(w).Encode(scaleReceiverState{s.attempts, s.accepted, s.ids, info.Size()})
			case "/lab/start":
				var input struct{ At time.Time }
				if r.Method != "POST" || json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&input) != nil {
					w.WriteHeader(400)
					return
				}
				s.recoverAt = input.At
				w.WriteHeader(204)
			default:
				w.WriteHeader(404)
			}
			return
		}
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
		var event scaleReceiverRecord
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&event); err != nil || event.Data.ID == "" {
			w.WriteHeader(400)
			return
		}
		// Event IDs are workspace-scoped. Separate receiver paths represent
		// independent customer destinations without changing production payloads.
		event.Workspace = strings.TrimPrefix(r.URL.Path, "/workspace/")
		if !accountID.MatchString(event.Workspace) {
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
		s.ids[scaleReceiverKey(event.Workspace, event.Data.ID)] = true
		s.accepted++
		w.WriteHeader(204)
	})
	s.server = httptest.NewUnstartedServer(handler)
	if listen := os.Getenv("DING_SCALE_RECEIVER_LISTEN"); os.Getenv("DING_SCALE_RECEIVER_ONLY") == "1" && listen != "" {
		s.server.Listener.Close()
		s.server.Listener, err = net.Listen("tcp", listen)
		if err != nil {
			t.Fatal(err)
		}
	}
	s.server.StartTLS()
	s.client = s.server.Client()
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
	if s.remoteToken != "" {
		s.remoteCall(t, "POST", "/lab/start", struct{ At time.Time }{at}, nil)
		return
	}
	s.mu.Lock()
	s.recoverAt = at
	s.mu.Unlock()
}
func (s *scaleReceiver) snapshot(t *testing.T) (scaleReceiverState, []byte) {
	t.Helper()
	if s.remoteToken != "" {
		return s.remoteSnapshot(t)
	}
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
