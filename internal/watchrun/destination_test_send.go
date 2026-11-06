package watchrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/notify"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

type DestinationTestRequest struct {
	AccountID      string `json:"accountID,omitempty"`
	WorkspaceID    string `json:"workspaceID,omitempty"`
	OperationKey   string `json:"operationKey"`
	DestinationRef string `json:"destinationRef"`
	Revision       string `json:"revision"`
}

// TestDestination sends at most one labeled attempt for a persisted operation.
// It resolves credentials on the host and never returns them in the receipt.
func (a *App) TestDestination(ctx context.Context, request DestinationTestRequest) (store.DestinationProbe, error) {
	var probe store.DestinationProbe
	if err := access.Require(ctx, access.Destinations); err != nil {
		return probe, err
	}
	principal, _ := access.FromContext(ctx)
	if (request.AccountID != "" || request.WorkspaceID != "") && !interactiveDestinationPrincipal(principal) {
		return probe, access.ErrDenied
	}
	if request.AccountID != "" && request.AccountID != principal.User || request.WorkspaceID != "" && request.WorkspaceID != principal.Workspace {
		return probe, access.ErrDenied
	}
	destination, endpoint, headers, err := a.destinationMaterial(ctx, request.DestinationRef)
	if err != nil {
		return probe, err
	}
	if request.Revision == "" || request.Revision != destination.Revision {
		return probe, store.ErrConflict
	}
	spec := destination.Definition.Spec
	identity, err := json.Marshal([]any{principal.User, principal.Grant, destination, endpoint, headers})
	if err != nil {
		return probe, err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(identity))
	var start bool
	err = a.Store.Update(ctx, func(tx *store.Tx) error {
		current, err := tx.Destination(request.DestinationRef, "")
		if err != nil {
			return err
		}
		if current.Revision != request.Revision {
			return store.ErrConflict
		}
		if interactiveDestinationPrincipal(principal) {
			setup := destinationBinding(principal, destination, endpoint, headers)
			probe, start, err = tx.BeginSetupProbe(request.OperationKey, digest, setup, a.Now())
		} else {
			probe, start, err = tx.BeginDestinationProbe(request.OperationKey, digest, a.Now())
		}
		return err
	})
	if err != nil || !start {
		return probe, err
	}
	event := watch.Event{ID: "ding-test-" + request.OperationKey, WatchID: "ding-setup-test", Type: "test", At: probe.At, Message: "Ding setup test. This is a labeled delivery test, not an incident."}
	payload, err := delivery.Render(spec.Type, event)
	if err != nil {
		return probe, fmt.Errorf("destination test format unavailable")
	}
	attempt, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result := delivery.Result{Outcome: delivery.Permanent}
	switch spec.Type {
	case "console":
		result = a.console(attempt, payload)
	case "desktop":
		if a.Notify != nil {
			if err := a.Notify(attempt, notify.Message{ID: event.ID, Title: "Ding setup test", Body: event.Message, RequestPermission: true}); err == nil {
				result.Outcome = delivery.Delivered
			} else {
				result.Outcome = delivery.Retryable
			}
		}
	default:
		headers.Set("Idempotency-Key", event.ID)
		headers.Set("X-Ding-Event-Id", event.ID)
		client := a.HTTP.Client
		if client == nil {
			client = http.DefaultClient
		}
		result = delivery.HTTP(attempt, client, delivery.Request{URL: endpoint, Body: payload, Header: headers, Provider: spec.Type}, a.Now())
	}
	// Transport loss may follow acceptance. Preserve uncertainty instead of
	// automatically resending a notification that somebody may already have seen.
	if result.Outcome == delivery.Retryable {
		return probe, nil
	}
	outcome := "failed"
	if result.Outcome == delivery.Delivered {
		outcome = "accepted"
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	if err := a.Store.Update(finishCtx, func(tx *store.Tx) error { return tx.FinishDestinationProbe(request.OperationKey, outcome) }); err != nil {
		return probe, err
	}
	probe.Outcome = outcome
	return probe, nil
}
