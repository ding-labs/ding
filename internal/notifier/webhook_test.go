package notifier_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/notifier"
)

// makeRunAlert is defined in slack_test.go (same package); reused here.

func TestWebhookNotifier_Drain_waitsForInFlightDelivery(t *testing.T) {
	delivered := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		select {
		case delivered <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	n := notifier.NewWebhookNotifier(srv.URL, 3, 1*time.Millisecond, nil)

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	n.Drain(2 * time.Second)
	elapsed := time.Since(start)

	select {
	case <-delivered:
	default:
		t.Fatalf("Drain returned before delivery completed (elapsed=%v)", elapsed)
	}
	if elapsed < 350*time.Millisecond {
		t.Errorf("Drain returned in %v; should have waited for in-flight delivery (~400ms)", elapsed)
	}
}

func TestWebhookNotifier_Drain_respectsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer srv.Close()

	n := notifier.NewWebhookNotifier(srv.URL, 1, 1*time.Millisecond, nil)
	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	n.Drain(200 * time.Millisecond)
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("Drain took %v; should have respected the 200ms timeout", elapsed)
	}
}
