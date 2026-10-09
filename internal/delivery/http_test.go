package delivery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, provider, body string
		status               int
		want                 Outcome
	}{
		{"ok", "webhook", "", 200, Delivered}, {"no-content", "discord", "", 204, Delivered},
		{"unauthorized", "webhook", "", 401, Permanent}, {"forbidden", "webhook", "", 403, Permanent},
		{"timeout", "webhook", "", 408, Retryable}, {"rate-limit", "webhook", "", 429, Retryable},
		{"unavailable", "webhook", "", 503, Retryable}, {"redirect", "webhook", "", 302, Permanent},
		{"slack-ok", "slack", "ok", 200, Delivered}, {"slack-reject", "slack", "invalid_payload", 200, Permanent},
		{"telegram-ok", "telegram", `{"ok":true}`, 200, Delivered}, {"telegram-reject", "telegram", `{"ok":false,"error_code":403}`, 200, Permanent},
		{"telegram-http-rate", "telegram", `{"ok":false,"error_code":429,"parameters":{"retry_after":9}}`, 429, Retryable},
		{"telegram-rate", "telegram", `{"ok":false,"error_code":429,"parameters":{"retry_after":9}}`, 200, Retryable},
		{"telegram-broken", "telegram", `garbage`, 200, Retryable},
		{"pd-ok", "pagerduty", `{"status":"success"}`, 202, Delivered}, {"pd-reject", "pagerduty", `{"status":"invalid"}`, 202, Permanent},
		{"pd-broken", "pagerduty", `garbage`, 202, Retryable},
		{"large", "webhook", strings.Repeat("x", 65537), 200, Retryable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "3")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			got := HTTP(context.Background(), srv.Client(), Request{URL: srv.URL, Provider: tc.provider}, now)
			if got.Outcome != tc.want {
				t.Fatalf("%+v", got)
			}
			if tc.status == 429 && tc.provider != "telegram" && !got.RetryAt.Equal(now.Add(3*time.Second)) {
				t.Fatal(got)
			}
			if strings.HasPrefix(tc.name, "telegram-") && strings.Contains(tc.name, "rate") && !got.RetryAt.Equal(now.Add(9*time.Second)) {
				t.Fatal(got)
			}
		})
	}
}
func TestTransportRedactionAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := HTTP(ctx, &http.Client{}, Request{URL: "https://secret-token@example.invalid"}, time.Now())
	if got.Outcome != Retryable || strings.Contains(got.Detail, "secret") {
		t.Fatal(got)
	}
	got = HTTP(ctx, &http.Client{}, Request{URL: "://secret"}, time.Now())
	if got.Outcome != Permanent {
		t.Fatal(got)
	}
}
func TestRetryAfterAndBackoff(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, v := range []string{"nonsense", "-1", "999999999999999999999"} {
		if !RetryAfter(v, now).IsZero() {
			t.Fatal(v)
		}
	}
	if !RetryAfter(now.Add(time.Minute).Format(http.TimeFormat), now).Equal(now.Add(time.Minute)) {
		t.Fatal("date")
	}
	if RetryAfter("9999999999", now).Sub(now) != 365*24*time.Hour {
		t.Fatal("bound")
	}
	if Backoff(0, 2) != 2*time.Second || Backoff(time.Hour, 80) != time.Hour {
		t.Fatal("backoff")
	}
}

func TestDiscordFractionalRetryDeadline(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"retry_after":2.75,"global":false}`))
	}))
	defer server.Close()
	result := HTTP(context.Background(), server.Client(), Request{URL: server.URL, Provider: "discord"}, now)
	if result.Outcome != Retryable || !result.RetryAt.Equal(now.Add(2750*time.Millisecond)) {
		t.Fatal(result)
	}
}
