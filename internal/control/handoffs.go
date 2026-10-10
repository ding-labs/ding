package control

import (
	"errors"
	"github.com/ding-labs/ding/internal/hostingpolicy"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"net/http"
)

func handoffRoutes(mux *http.ServeMux, app *watchrun.App) {
	reply := func(w http.ResponseWriter, value any, err error) {
		if errors.Is(err, store.ErrNotFound) {
			fail(w, 404, "not_found", "transfer or watch not found")
		} else if err != nil {
			fail(w, 409, "handoff_conflict", err.Error())
		} else {
			write(w, 200, value, nil)
		}
	}
	mux.HandleFunc("GET /v1/handoffs/preflight/{watch}", func(w http.ResponseWriter, r *http.Request) {
		out, err := app.PreflightHandoff(r.Context(), r.PathValue("watch"))
		if err == nil && out.Ready {
			b, e := plan.Parse([]byte(out.Manifest))
			if e == nil {
				e = hostingpolicy.Policy(b)
			}
			if e != nil {
				out.Ready = false
				out.Reason = e.Error()
			}
		}
		reply(w, out, err)
	})
	mux.HandleFunc("GET /v1/handoffs", func(w http.ResponseWriter, r *http.Request) {
		var out []store.Handoff
		err := app.Store.View(r.Context(), func(tx *store.Tx) error { var e error; out, e = tx.Handoffs(); return e })
		reply(w, out, err)
	})
	mux.HandleFunc("GET /v1/handoffs/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, err := app.Handoff(r.Context(), r.PathValue("id"))
		reply(w, out, err)
	})
	mux.HandleFunc("POST /v1/handoffs/prepare", func(w http.ResponseWriter, r *http.Request) {
		var input watchrun.HandoffPrepare
		if decode(w, r, &input) != nil {
			fail(w, 400, "invalid_handoff", "provide an exact prepared transfer")
			return
		}
		out, err := app.PrepareHandoff(r.Context(), input)
		reply(w, out, err)
	})
	mux.HandleFunc("POST /v1/handoffs/pause", func(w http.ResponseWriter, r *http.Request) {
		var input watchrun.HandoffPause
		if decode(w, r, &input) != nil {
			fail(w, 400, "invalid_handoff", "provide the reviewed source generation")
			return
		}
		out, err := app.PauseForHandoff(r.Context(), input)
		reply(w, out, err)
	})
	mux.HandleFunc("POST /v1/handoffs/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			OperationKey string `json:"operationKey"`
		}
		if decode(w, r, &input) != nil {
			fail(w, 400, "invalid_test", "provide a stable operation key")
			return
		}
		out, err := app.TestHandoff(r.Context(), r.PathValue("id"), input.OperationKey)
		reply(w, out, err)
	})
	for _, action := range []string{"activate", "release", "cancel"} {
		mux.HandleFunc("POST /v1/handoffs/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			var proof store.Handoff
			if action != "cancel" && decode(w, r, &proof) != nil {
				fail(w, 400, "invalid_handoff", "a peer receipt is required")
				return
			}
			var out store.Handoff
			var err error
			switch action {
			case "activate":
				out, err = app.ActivateHandoff(r.Context(), r.PathValue("id"), proof)
			case "release":
				out, err = app.ReleaseHandoff(r.Context(), r.PathValue("id"), proof)
			case "cancel":
				out, err = app.CancelPreparedHandoff(r.Context(), r.PathValue("id"))
			}
			reply(w, out, err)
		})
	}
}
