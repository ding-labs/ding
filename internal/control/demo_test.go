package control

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/demo"
)

func TestDemoStreamCancelsAndReleasesSingleSlot(t *testing.T) {
	var calls atomic.Int32
	finished := make(chan struct{})
	mux := http.NewServeMux()
	registerDemo(mux, func(ctx context.Context, options demo.Options) error {
		if calls.Add(1) == 1 {
			<-ctx.Done()
			close(finished)
			return ctx.Err()
		}
		return nil
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/v1/demo", strings.NewReader(`{}`))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	blocked, err := http.Post(server.URL+"/v1/demo", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	blocked.Body.Close()
	if blocked.StatusCode != 429 {
		t.Fatal(blocked.StatusCode)
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not cancel demo")
	}
	// Wait for the handler's defer to return the slot, not just the runner.
	for deadline := time.Now().Add(3 * time.Second); ; {
		r, err := http.Post(server.URL+"/v1/demo", "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode == 200 {
			if !strings.Contains(string(body), `"stage":"complete"`) {
				t.Fatal(string(body))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("demo slot was not released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
