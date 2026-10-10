package control

import (
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"net/http"
	"strconv"
)

func consoleQuery(r *http.Request) (store.ConsoleQuery, error) {
	v := r.URL.Query()
	q := store.ConsoleQuery{Search: v.Get("search"), Status: v.Get("status"), Source: v.Get("source"), Attention: v.Get("attention"), Incident: v.Get("incident"), Health: v.Get("health"), Delivery: v.Get("delivery"), Watch: v.Get("watch"), Event: v.Get("event"), Destination: v.Get("destination"), Type: v.Get("type"), From: v.Get("from"), To: v.Get("to"), Cursor: v.Get("cursor"), Limit: 50}
	if v.Get("limit") != "" {
		var err error
		q.Limit, err = strconv.Atoi(v.Get("limit"))
		if err != nil {
			return q, store.ErrCursor
		}
	}
	return q, store.ValidateConsoleQuery(q)
}
func consoleRoutes(mux *http.ServeMux, app *watchrun.App, cfg ConsoleConfig) {
	updateRoutes(mux, cfg)
	toolRoutes(mux, app)
	onboardingRoutes(mux, app)
	systemRoutes(mux, app, cfg)
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) {
		s, err := app.Status(r.Context())
		if err != nil {
			inspectionError(w, err)
			return
		}
		write(w, 200, s, nil)
	})
	for _, kind := range []string{"watches", "events", "deliveries", "destinations"} {
		mux.HandleFunc("GET /v1/console/"+kind, func(w http.ResponseWriter, r *http.Request) {
			q, err := consoleQuery(r)
			if err != nil {
				inspectionError(w, err)
				return
			}
			var page any
			if kind == "watches" {
				page, err = app.ConsoleWatches(r.Context(), q)
			} else {
				err = app.Store.View(r.Context(), func(tx *store.Tx) error {
					var e error
					switch kind {
					case "events":
						page, e = tx.ConsoleEvents(q)
					case "deliveries":
						page, e = tx.ConsoleDeliveries(q)
					case "destinations":
						page, e = tx.ConsoleDestinations(q)
					}
					return e
				})
			}
			if err != nil {
				inspectionError(w, err)
				return
			}
			write(w, 200, page, nil)
		})
	}
}
