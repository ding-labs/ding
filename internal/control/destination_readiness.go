package control

import (
	"errors"
	"net/http"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
	"gopkg.in/yaml.v3"
)

func destinationSetupError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrOperationConflict) {
		fail(w, 409, "setup_changed", "The setup changed or expired. Refresh the destination and send a new labeled test if needed.")
	} else if errors.Is(err, access.ErrDenied) || errors.Is(err, store.ErrNotFound) {
		inspectionError(w, err)
	} else {
		fail(w, 503, "setup_unavailable", "Destination setup is unavailable. Refresh its status before retrying.")
	}
}

func DestinationSetupStatus(w http.ResponseWriter, r *http.Request, app *watchrun.App) {
	if len(r.URL.Query().Get("destination")) > 128 {
		fail(w, 400, "invalid_destination", "Use a saved destination ID.")
		return
	}
	out, err := app.DestinationSetupStatus(r.Context(), r.URL.Query().Get("destination"))
	if err != nil {
		destinationSetupError(w, err)
		return
	}
	write(w, 200, out, nil)
}

func destinationSetupRoutes(mux *http.ServeMux, app *watchrun.App) {
	mux.HandleFunc("GET /v1/onboarding/destination-status", func(w http.ResponseWriter, r *http.Request) { DestinationSetupStatus(w, r, app) })
	mux.HandleFunc("POST /v1/onboarding/destination-confirm", func(w http.ResponseWriter, r *http.Request) {
		var req watchrun.DestinationTestRequest
		if decodeBounded(w, r, &req, 8192) != nil {
			fail(w, 400, "invalid_confirmation", "Use the reviewed destination and accepted test receipt.")
			return
		}
		out, err := app.ConfirmDestinationReceipt(r.Context(), req)
		if err != nil {
			destinationSetupError(w, err)
			return
		}
		write(w, 200, out, nil)
	})
	mux.HandleFunc("POST /v1/onboarding/destination", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AccountID   string `json:"accountID"`
			WorkspaceID string `json:"workspaceID"`
			ID          string `json:"id"`
			Type        string `json:"type"`
			Credential  string `json:"credential"`
		}
		if decodeBounded(w, r, &req, 8192) != nil {
			fail(w, 400, "invalid_destination", "Provide a destination ID, type and credential reference.")
			return
		}
		p, ok := access.FromContext(r.Context())
		if !ok || p.Grant != "" || (p.Kind != "browser" && p.Kind != "personal") || !p.Allows(access.Destinations) || p.User != req.AccountID || p.Workspace != req.WorkspaceID {
			inspectionError(w, access.ErrDenied)
			return
		}
		d := watch.Destination{APIVersion: watch.APIVersion, Kind: "Destination", Metadata: watch.Metadata{ID: req.ID}, Spec: watch.DestinationSpec{Type: req.Type}}
		if req.Type != "console" && req.Type != "desktop" {
			d.Spec.URLRef = &watch.SecretRef{Env: req.Credential}
		} else if req.Credential != "" {
			fail(w, 400, "invalid_destination", "This destination does not use a credential.")
			return
		}
		manifest, err := yaml.Marshal(d)
		if err != nil {
			destinationSetupError(w, err)
			return
		}
		review, err := app.Apply(r.Context(), watchrun.ApplyRequest{Manifest: string(manifest), DryRun: true})
		if err != nil {
			fail(w, 400, "invalid_destination", "Check the destination ID, type, credential reference and execution policy.")
			return
		}
		if len(review.Changes) != 0 || len(review.DestinationChanges) != 1 || review.DestinationChanges[0].State != "created" {
			destinationSetupError(w, store.ErrConflict)
			return
		}
		out, err := app.Apply(r.Context(), watchrun.ApplyRequest{Manifest: string(manifest), Review: review.Review})
		if err != nil {
			destinationSetupError(w, err)
			return
		}
		write(w, 201, out, nil)
	})
}
