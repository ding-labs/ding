package control

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProducerRetriesSamePayloadAndKey(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer ingest" || r.Header.Get("Idempotency-Key") != "snapshot-1" || string(body) != `{"value":1}` {
			t.Error("request identity changed")
		}
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		write(w, 202, map[string]int{"first": 1, "last": 1}, nil)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c := Client{URL: server.URL, Token: "ingest", HTTP: server.Client()}
	if err := c.Produce(ctx, "/v1/ingest/test", "snapshot-1", map[string]int{"value": 1}); err != nil || calls != 2 {
		t.Fatal(err, calls)
	}
}

func TestProducerDoesNotRetryPermanentRejection(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(409) }))
	defer server.Close()
	c := Client{URL: server.URL, HTTP: server.Client()}
	if err := c.Produce(context.Background(), "/input", "key", nil); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
