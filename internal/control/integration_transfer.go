package control

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"

	"github.com/ding-labs/ding/internal/watchrun"
)

// TransferStep is a closed transport for the trusted coordinator. Public model
// tools expose reviewed prepare/test/finish/cancel operations, never peer proofs.
// The caller must authenticate the integration grant and install its principal.
func TransferStep(w http.ResponseWriter, r *http.Request, app *watchrun.App, next http.Handler) {
	var input struct {
		Action string          `json:"action"`
		ID     string          `json:"transfer_id"`
		Input  json.RawMessage `json:"input"`
	}
	if decodeBounded(w, r, &input, int64(app.Integration.MaxRequestBytes)) != nil {
		fail(w, 400, "invalid_transfer", "provide a bounded transfer step")
		return
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(input.ID) {
		fail(w, 400, "invalid_transfer", "use the original transfer ID")
		return
	}
	var path, manifest, watchID string
	switch input.Action {
	case "prepare":
		var p watchrun.HandoffPrepare
		if json.Unmarshal(input.Input, &p) != nil || p.ID != input.ID {
			fail(w, 400, "invalid_transfer", "preparation must match the transfer ID")
			return
		}
		manifest = p.Manifest
		path = "/v1/handoffs/prepare"
	case "pause":
		var p watchrun.HandoffPause
		if json.Unmarshal(input.Input, &p) != nil || p.ID != input.ID {
			fail(w, 400, "invalid_transfer", "pause must match the transfer ID")
			return
		}
		watchID = p.WatchID
		path = "/v1/handoffs/pause"
	case "test", "activate", "cancel", "release":
		h, err := app.Handoff(r.Context(), input.ID)
		if err != nil {
			integrationError(w, err)
			return
		}
		watchID = h.WatchID
		path = "/v1/handoffs/" + input.ID + "/" + input.Action
	default:
		fail(w, 400, "invalid_transfer", "unsupported transfer step")
		return
	}
	if err := app.AuthorizeIntegrationHandoff(r.Context(), manifest, watchID); err != nil {
		integrationError(w, err)
		return
	}
	forward := r.Clone(r.Context())
	forward.URL.Path = path
	forward.URL.RawPath = ""
	forward.Body = io.NopCloser(bytes.NewReader(input.Input))
	forward.ContentLength = int64(len(input.Input))
	next.ServeHTTP(w, forward)
}
