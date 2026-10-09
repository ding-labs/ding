package control

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func inspectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrCursor):
		fail(w, 400, "invalid_cursor", "cursor is invalid for this store or watch")
	case errors.Is(err, store.ErrCursorExpired):
		fail(w, 410, "cursor_expired", "retained history has a gap; restart without a cursor to read available events")
	case errors.Is(err, store.ErrNotFound):
		fail(w, 404, "not_found", "record not found")
	case errors.Is(err, store.ErrConflict):
		fail(w, 409, "delivery_not_terminal", "only failed or canceled deliveries can be retried")
	case errors.Is(err, watchrun.ErrQuota):
		fail(w, 429, "quota_exceeded", "resource quota exceeded")
	case errors.Is(err, watchrun.ErrClosing):
		fail(w, 503, "shutting_down", "runtime is shutting down")
	default:
		fail(w, 503, "store_unavailable", "cannot complete store operation")
	}
}
func inspectionRoutes(mux *http.ServeMux, app *watchrun.App) {
	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			var err error
			limit, err = strconv.Atoi(v)
			if err != nil {
				inspectionError(w, store.ErrCursor)
				return
			}
		}
		// Alpha's numeric offset was not retention-aware. Refuse it explicitly.
		if r.URL.Query().Has("after") {
			fail(w, 400, "invalid_cursor", "use cursor from the previous event page")
			return
		}
		var page store.EventPage
		err := app.Store.View(r.Context(), func(tx *store.Tx) error {
			var err error
			page, err = tx.EventPage(r.URL.Query().Get("watch"), r.URL.Query().Get("cursor"), limit)
			return err
		})
		if err != nil {
			inspectionError(w, err)
			return
		}
		write(w, 200, page, nil)
	})
	mux.HandleFunc("GET /v1/events/{id}", func(w http.ResponseWriter, r *http.Request) {
		proof, err := app.Evidence(r.Context(), r.PathValue("id"))
		if err != nil {
			inspectionError(w, err)
			return
		}
		write(w, 200, proof, nil)
	})
	mux.HandleFunc("GET /v1/events/{id}/observations", func(w http.ResponseWriter, r *http.Request) {
		after, err := nonnegative(r.URL.Query().Get("after"))
		if err != nil {
			inspectionError(w, store.ErrCursor)
			return
		}
		var page store.ObservationPage
		err = app.Store.View(r.Context(), func(tx *store.Tx) error {
			var err error
			page, err = tx.EvidenceInputs(r.PathValue("id"), after)
			return err
		})
		if err != nil {
			inspectionError(w, err)
			return
		}
		write(w, 200, page, nil)
	})
	mux.HandleFunc("GET /v1/doctor", func(w http.ResponseWriter, r *http.Request) {
		d, err := app.Doctor(r.Context())
		if err != nil {
			inspectionError(w, err)
			return
		}
		write(w, 200, d, nil)
	})
	mux.HandleFunc("GET /v1/watches/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		manifest, err := app.Export(r.Context(), r.PathValue("id"))
		if err != nil {
			inspectionError(w, err)
			return
		}
		write(w, 200, map[string]string{"manifest": manifest}, nil)
	})
	mux.HandleFunc("POST /v1/backup", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Path string `json:"path"`
		}
		if decode(w, r, &request) != nil || !filepath.IsAbs(request.Path) {
			fail(w, 400, "invalid_request", "backup requires an absolute output path on the daemon host")
			return
		}
		if err := app.Store.Backup(r.Context(), request.Path); err != nil {
			fail(w, 409, "backup_failed", "backup could not be created; use an unused path in an existing writable directory")
			return
		}
		write(w, 200, map[string]any{"path": request.Path, "verified": true}, nil)
	})
	mux.HandleFunc("GET /v1/deliveries/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		before, e := nonnegative(r.URL.Query().Get("before"))
		if err != nil || e != nil || id < 1 {
			fail(w, 400, "invalid_request", "invalid delivery ID or attempt cursor")
			return
		}
		var d store.DeliveryInspection
		err = app.Store.View(r.Context(), func(tx *store.Tx) error { var err error; d, err = tx.Delivery(id, before); return err })
		if err != nil {
			inspectionError(w, err)
			return
		}
		write(w, 200, d, nil)
	})
	mux.HandleFunc("POST /v1/deliveries/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			fail(w, 400, "invalid_request", "invalid delivery ID")
			return
		}
		if err := app.Retry(r.Context(), id); err != nil {
			inspectionError(w, err)
			return
		}
		write(w, 200, map[string]any{"id": id, "status": "pending"}, nil)
	})
}
func nonnegative(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, store.ErrCursor
	}
	return n, nil
}
