package control

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ding-labs/ding/internal/demo"
)

func demoRoutes(mux *http.ServeMux) { registerDemo(mux, demo.Run) }

func registerDemo(mux *http.ServeMux, run func(context.Context, demo.Options) error) {
	slot := make(chan struct{}, 1)
	mux.HandleFunc("POST /v1/demo", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Desktop bool `json:"desktop"`
		}
		if decodeBounded(w, r, &request, 1024) != nil {
			fail(w, 400, "invalid_request", "expected a desktop notification choice")
			return
		}
		select {
		case slot <- struct{}{}:
			defer func() { <-slot }()
		default:
			fail(w, 429, "demo_busy", "A demonstration is already running. Wait for it to finish or close its dialog.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 110*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		encoder := json.NewEncoder(w)
		emit := func(update demo.Update) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := encoder.Encode(update); err != nil {
				return err
			}
			return http.NewResponseController(w).Flush()
		}
		if err := emit(demo.Update{Stage: "starting"}); err != nil {
			return
		}
		err := run(ctx, demo.Options{Desktop: request.Desktop, Progress: emit})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			_ = emit(demo.Update{Stage: "failed", Error: err.Error()})
			return
		}
		// Run returns after its temporary runtime, fixture and state are closed.
		_ = emit(demo.Update{Stage: "complete"})
	})
}
