package watchcli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestLogPublisherReconnectsWithFreshEvidence(t *testing.T) {
	connected := make(chan watchrun.LogSnapshot, 1)
	config := watch.LogSource{Window: "1s", MinEvents: 1}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/logs/test/session" {
			json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Data: watchrun.LogSession{ID: "new", Config: config}})
			return
		}
		var input watchrun.LogSnapshot
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if input.Session == "old" {
			w.WriteHeader(409)
			return
		}
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Data: watchrun.Receipt{First: 1, Last: 1}})
		if !input.Ended {
			select {
			case connected <- input:
			default:
			}
		}
	}))
	defer server.Close()
	c, _ := logs.NewCollector(logs.ParserConfig{}, logs.WindowConfig{Window: time.Second, MinEvents: 1})
	defer c.Close()
	finish := publishLogs(context.Background(), control.Client{URL: server.URL, HTTP: server.Client()}, "test", c, io.Discard, &watchrun.LogSession{ID: "old", Config: config})
	defer finish(nil)
	select {
	case input := <-connected:
		if input.Session != "new" || input.Sequence != 1 || input.Summary.Ready || input.Summary.Reason != "warming_up" {
			t.Fatal(input)
		}
	case <-time.After(9 * time.Second):
		t.Fatal("collector did not reconnect")
	}
}
