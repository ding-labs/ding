package egress

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type hostTransport func(*http.Request) (*http.Response, error)

func (f hostTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestHotHostDoesNotConsumeAnotherHostsAdmission(t *testing.T) {
	next := hostTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{Body: io.NopCloser(strings.NewReader("ok")), StatusCode: 200}, nil
	})
	h := NewHostLimits(NewLimit(next, 1))
	for i := 0; i < 2; i++ {
		r, _ := http.NewRequest("GET", "https://hot.example", nil)
		resp, err := h.RoundTrip(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", "https://hot.example", nil)
	if _, err := h.RoundTrip(r); err == nil {
		t.Fatal("hot host exceeded rate")
	}
	r, _ = http.NewRequest("GET", "https://quiet.example", nil)
	resp, err := h.RoundTrip(r)
	if err != nil {
		t.Fatal("other host blocked", err)
	}
	resp.Body.Close()
}
