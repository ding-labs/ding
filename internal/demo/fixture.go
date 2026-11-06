package demo

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// HealthFixture is a synthetic service for local first-watch qualification.
// Serve it on loopback only: its mode control intentionally has no credentials.
func HealthFixture() http.Handler {
	var mode atomic.Value
	mode.Store("healthy")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load().(string) {
		case "timeout":
			<-r.Context().Done()
			return
		case "failing":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusOK)
		}
		_, _ = io.WriteString(w, "synthetic Ding health fixture\n")
	})
	mux.HandleFunc("GET /mode", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, mode.Load().(string)+"\n")
	})
	mux.HandleFunc("PUT /mode", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32))
		if err != nil {
			http.Error(w, "mode must fit in 32 bytes", http.StatusBadRequest)
			return
		}
		value := strings.TrimSpace(string(body))
		if value != "healthy" && value != "failing" && value != "timeout" {
			http.Error(w, "choose healthy, failing, or timeout", http.StatusBadRequest)
			return
		}
		mode.Store(value)
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}
