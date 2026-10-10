package cloud

import (
	"context"
	"net/http"
	"time"

	"github.com/ding-labs/ding/internal/cloud/egress"
	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/watch"
)

type DestinationTest struct {
	OperationKey string `json:"operationKey"`
	Destination  string `json:"destination"`
	Credential   string `json:"credential"`
}

func (s *Server) testDestination(w http.ResponseWriter, r *http.Request, t *Tenant) {
	var req DestinationTest
	if decodeCloud(r, &req) != nil || (req.Destination != "webhook" && req.Destination != "slack" && req.Destination != "discord") {
		cloudFail(w, 400, "invalid_test", "Provide a stable operation key, destination type and credential reference.")
		return
	}
	endpoint, revision, err := s.Vault.GetWithRevision(r.Context(), t.Account.ID, req.Credential)
	if err != nil {
		cloudFail(w, 400, "missing_credential", "Save this workspace's destination credential before testing.")
		return
	}
	if err := egress.URL(endpoint); err != nil {
		cloudFail(w, 400, "destination_denied", "Cloud delivery requires a public HTTP(S) destination on its standard port.")
		return
	}
	digest := state.TokenHash(req.Destination + "\x00" + req.Credential + "\x00" + revision)
	probe, start, err := s.DB.BeginProbe(r.Context(), t.Account.ID, req.OperationKey, digest)
	if err != nil {
		cloudFail(w, 409, "test_conflict", err.Error())
		return
	}
	if start {
		id := state.TokenHash(t.Account.ID + "\x00" + req.OperationKey)
		payload, err := delivery.Render(req.Destination, watch.Event{ID: id, WatchID: "ding-cloud-test", Type: "test", At: time.Now().UTC(), Message: "Ding Cloud delivery test. This is a labeled setup test, not an incident."})
		if err != nil {
			cloudFail(w, 400, "test_failed", "Could not render the destination test.")
			return
		}
		result := delivery.HTTP(r.Context(), t.App.HTTP.Client, delivery.Request{URL: endpoint, Body: payload, Header: http.Header{"Idempotency-Key": []string{id}}, Provider: req.Destination}, time.Now())
		outcome := "failed"
		if result.Outcome == delivery.Delivered {
			outcome = "accepted"
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer cancel()
		if err := s.DB.FinishProbe(ctx, t.Account.ID, probe.ID, outcome); err != nil {
			cloudFail(w, 503, "test_outcome_unknown", "Test outcome is uncertain. Reconcile using the same operation key; do not automatically send another test.")
			return
		}
		probe.Outcome = outcome
	}
	cloudWrite(w, 200, probe)
}
