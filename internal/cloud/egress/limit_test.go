package egress

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSharedSlotHeldUntilResponseClosed(t *testing.T) {
	l := NewLimit(roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("body"))}, nil
	}), 1)
	r, _ := http.NewRequest("GET", "https://example.com", nil)
	first, err := l.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := l.RoundTrip(r.WithContext(ctx)); err == nil {
		t.Fatal("slot released before body consumption")
	}
	first.Body.Close()
	first.Body.Close()
	last, err := l.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	last.Body.Close()
}
