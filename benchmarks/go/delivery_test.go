package bench_test

import (
	"context"
	"github.com/ding-labs/ding/internal/delivery"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func BenchmarkHTTPDelivery(b *testing.B) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer s.Close()
	req := delivery.Request{URL: s.URL, Body: []byte(`{"event":"example"}`)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if result := delivery.HTTP(context.Background(), s.Client(), req, time.Now()); result.Outcome != delivery.Delivered {
			b.Fatal(result)
		}
	}
}
