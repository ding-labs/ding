package control

import (
	"context"
	"net/http"
	"time"

	"github.com/ding-labs/ding/internal/notify"
	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watchrun"
)

type FirstWatchPreview struct {
	Manifest     string               `json:"manifest"`
	Descriptions []plan.Description   `json:"descriptions"`
	Review       watchrun.ApplyResult `json:"review"`
}

func onboardingRoutes(mux *http.ServeMux, app *watchrun.App) {
	mux.HandleFunc("POST /v1/onboarding/preview", func(w http.ResponseWriter, r *http.Request) {
		var req onboarding.Request
		if decodeBounded(w, r, &req, 8192) != nil {
			fail(w, 400, "invalid_request", "expected watch ID, URL and delivery choice")
			return
		}
		manifest, desc, err := onboarding.Manifest(req)
		if err != nil {
			fail(w, 400, "invalid_watch", err.Error())
			return
		}
		review, err := app.Apply(r.Context(), watchrun.ApplyRequest{Manifest: manifest, DryRun: true})
		if err != nil {
			fail(w, 400, "preview_failed", err.Error())
			return
		}
		for _, change := range append(review.Changes, review.DestinationChanges...) {
			if change.State != "created" {
				fail(w, 409, "already_exists", "choose a new watch ID; edit existing watches in Workbench")
				return
			}
		}
		write(w, 200, FirstWatchPreview{Manifest: manifest, Descriptions: desc, Review: review}, nil)
	})
	slot := make(chan struct{}, 1)
	mux.HandleFunc("POST /v1/desktop/test", func(w http.ResponseWriter, r *http.Request) {
		if app.Notify == nil {
			fail(w, 503, "desktop_unavailable", "desktop delivery is not enabled on this instance")
			return
		}
		select {
		case slot <- struct{}{}:
			defer func() { <-slot }()
		default:
			fail(w, 429, "notification_busy", "a notification test is already running")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		err := app.Notify(ctx, notify.Message{ID: randomToken(), Title: "Ding test notification", Body: "Your computer accepted this test. Real alerts will identify their watch and event.", RequestPermission: true})
		if err != nil {
			fail(w, 503, "notification_unavailable", err.Error())
			return
		}
		write(w, 200, map[string]any{"accepted": true, "meaning": "OS accepted the test; confirm that you can see it."}, nil)
	})
}
