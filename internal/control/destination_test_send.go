package control

import (
	"errors"
	"net/http"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

// TestSavedDestination shares the local and hosted receipt/error contract.
func TestSavedDestination(w http.ResponseWriter, r *http.Request, app *watchrun.App, request watchrun.DestinationTestRequest) {
	if request.OperationKey == "" || request.DestinationRef == "" || request.Revision == "" {
		fail(w, 400, "invalid_test", "Provide an operation key, saved destination ID, and reviewed revision.")
		return
	}
	probe, err := app.TestDestination(r.Context(), request)
	switch {
	case err == nil:
		write(w, 200, probe, nil)
	case errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrOperationConflict):
		fail(w, 409, "test_conflict", "The destination, credentials, or request changed. Preview again before sending a new test.")
	case errors.Is(err, access.ErrDenied), errors.Is(err, store.ErrNotFound):
		inspectionError(w, err)
	default:
		fail(w, 503, "test_unavailable", "Check destination credentials and availability. Reuse the same test key to reconcile an uncertain outcome.")
	}
}
