package control

import (
	"errors"
	"net/http"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func logRoutes(mux *http.ServeMux, app *watchrun.App) {
	reply := func(w http.ResponseWriter, result any, err error, status int) {
		switch {
		case errors.Is(err, store.ErrNotFound):
			fail(w, 404, "not_found", "log watch not found")
		case errors.Is(err, watchrun.ErrInput):
			fail(w, 400, "invalid_input", "invalid log summary")
		case errors.Is(err, store.ErrStale), errors.Is(err, watchrun.ErrLogSessionBusy):
			fail(w, 409, "log_session_conflict", "collector session is inactive, replaced, or already connected")
		case errors.Is(err, watchrun.ErrQuota):
			w.Header().Set("Retry-After", "1")
			fail(w, 429, "quota_exceeded", "resource quota exceeded")
		case err != nil:
			fail(w, 503, "collector_unavailable", "log input is unavailable")
		default:
			write(w, status, result, nil)
		}
	}
	mux.HandleFunc("POST /v1/logs/{id}/session", func(w http.ResponseWriter, r *http.Request) {
		if err := app.Reserve(); err != nil {
			reply(w, nil, err, 0)
			return
		}
		defer app.Release()
		result, err := app.StartLogSession(r.Context(), r.PathValue("id"))
		reply(w, result, err, 200)
	})
	mux.HandleFunc("POST /v1/logs/{id}/summary", func(w http.ResponseWriter, r *http.Request) {
		if err := app.Reserve(); err != nil {
			reply(w, nil, err, 0)
			return
		}
		defer app.Release()
		var input watchrun.LogSnapshot
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := decode(w, r, &input); err != nil {
			reply(w, nil, watchrun.ErrInput, 0)
			return
		}
		result, err := app.AcceptLogSnapshot(r.Context(), r.PathValue("id"), input)
		reply(w, result, err, 202)
	})
}
