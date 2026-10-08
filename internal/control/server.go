// Package control exposes the local versioned API shared by CLI and agents.
package control

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func Handler(app *watchrun.App, c Credentials) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]string{"status": "running"}, nil)
	})
	mux.HandleFunc("POST /v1/apply", func(w http.ResponseWriter, r *http.Request) {
		var request watchrun.ApplyRequest
		if err := decode(w, r, &request); err != nil {
			fail(w, 400, "invalid_request", "invalid apply request")
			return
		}
		result, err := app.Apply(r.Context(), request)
		if errors.Is(err, watchrun.ErrQuota) {
			fail(w, 429, "quota_exceeded", "resource quota exceeded")
			return
		}
		if errors.Is(err, watchrun.ErrClosing) {
			fail(w, 503, "shutting_down", "runtime is shutting down")
			return
		}
		if errors.Is(err, store.ErrConflict) {
			fail(w, 409, "revision_conflict", "expected revision does not match")
			return
		}
		if err != nil {
			fail(w, 400, "apply_failed", err.Error())
			return
		}
		write(w, 200, result, nil)
	})
	mux.HandleFunc("GET /v1/watches", func(w http.ResponseWriter, r *http.Request) {
		result, err := app.List(r.Context())
		if err != nil {
			fail(w, 503, "store_unavailable", "cannot read store")
			return
		}
		write(w, 200, result, nil)
	})
	mux.HandleFunc("GET /v1/watches/{id}", func(w http.ResponseWriter, r *http.Request) {
		result, err := app.Inspect(r.Context(), r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			fail(w, 404, "not_found", "watch not found")
			return
		}
		if err != nil {
			fail(w, 503, "store_unavailable", "cannot read store")
			return
		}
		write(w, 200, result, nil)
	})
	mux.HandleFunc("POST /v1/watches/{id}/lifecycle", func(w http.ResponseWriter, r *http.Request) {
		var request watchrun.LifecycleRequest
		if err := decode(w, r, &request); err != nil {
			fail(w, 400, "invalid_request", "invalid lifecycle request")
			return
		}
		record, err := app.Lifecycle(r.Context(), r.PathValue("id"), request)
		switch {
		case errors.Is(err, store.ErrNotFound):
			fail(w, 404, "not_found", "watch not found")
		case errors.Is(err, store.ErrConflict):
			fail(w, 409, "revision_conflict", "expected revision does not match")
		case errors.Is(err, watchrun.ErrQuota):
			fail(w, 429, "quota_exceeded", "resource quota exceeded")
		case errors.Is(err, watchrun.ErrClosing):
			fail(w, 503, "shutting_down", "runtime is shutting down")
		case err != nil:
			fail(w, 400, "lifecycle_failed", err.Error())
		default:
			write(w, 200, record, nil)
		}
	})
	mux.HandleFunc("POST /v1/ingest/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := app.Reserve(); err != nil {
			fail(w, 503, "busy", "acquisition is unavailable")
			return
		}
		defer app.Release()
		record, err := app.Record(r.Context(), r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			fail(w, 404, "not_found", "watch not found")
			return
		}
		if err != nil {
			fail(w, 503, "store_unavailable", "cannot read store")
			return
		}
		if record.Plan.Definition.Spec.Source.Type != "push" || record.Status != "running" {
			fail(w, 409, "inactive_source", "watch is not an active push source")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(record.Plan.Definition.Spec.Limits.MaxBytes)))
		if err != nil {
			fail(w, 413, "input_limit", "input exceeds limit")
			return
		}
		receipt, err := app.IngestReserved(r.Context(), record, body, r.Header.Get("Idempotency-Key"))
		switch {
		case errors.Is(err, watchrun.ErrInput):
			fail(w, 400, "invalid_input", err.Error())
		case errors.Is(err, store.ErrStale):
			fail(w, 409, "stale_generation", "watch changed; retry against its current revision")
		case errors.Is(err, watchrun.ErrQuota):
			w.Header().Set("Retry-After", "1")
			fail(w, 429, "quota_exceeded", "resource quota exceeded")
		case err != nil:
			fail(w, 503, "acceptance_failed", "input was not accepted; retry later")
		default:
			write(w, 202, receipt, nil)
		}
	})
	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		after := int64(0)
		var err error
		if value := r.URL.Query().Get("after"); value != "" {
			after, err = strconv.ParseInt(value, 10, 64)
		}
		if err != nil || after < 0 {
			fail(w, 400, "invalid_cursor", "cursor must be nonnegative")
			return
		}
		var events []watch.Event
		err = app.Store.View(r.Context(), func(tx *store.Tx) error {
			var err error
			events, err = tx.Events(r.URL.Query().Get("watch"), after, 1000)
			return err
		})
		if err != nil {
			fail(w, 503, "store_unavailable", "cannot read events")
			return
		}
		write(w, 200, events, nil)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path != "/health" {
			want := c.Admin
			if strings.HasPrefix(r.URL.Path, "/v1/ingest/") {
				want = c.Ingest
			}
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if token == r.Header.Get("Authorization") || subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
				fail(w, 401, "unauthorized", "valid admin credential required")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func write(w http.ResponseWriter, status int, data any, err *watch.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Data: data, Error: err})
}
func fail(w http.ResponseWriter, status int, code, message string) {
	write(w, status, nil, &watch.Error{Code: code, Message: message})
}
func decode(w http.ResponseWriter, r *http.Request, target any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("extra input")
	}
	return nil
}
