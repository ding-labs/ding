package cloud

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

func (s *Server) cloudHandoff(w http.ResponseWriter, r *http.Request, t *Tenant) {
	s.moveMu.Lock()
	defer s.moveMu.Unlock() // Credentials cannot change between test-proof validation and activation.
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/handoffs/"), "/")
	if r.Method == "POST" && len(parts) == 2 && accountID.MatchString(parts[0]) {
		if parts[1] == "test" {
			s.testHandoff(w, r, t, parts[0])
			return
		}
		if parts[1] == "activate" {
			h, err := t.App.Handoff(r.Context(), parts[0])
			if err == nil && h.Phase != "active" {
				err = s.verifyHandoffProof(r.Context(), t.Account.ID, h)
			}
			if err != nil {
				cloudFail(w, 409, "test_required", "Test the prepared source and all destinations with their current credentials before activating.")
				return
			}
		}
	}
	api := s.api(t)
	copy := r.Clone(r.Context())
	copy.Header = r.Header.Clone()
	copy.Header.Set("Authorization", "Bearer "+api.token)
	api.handler.ServeHTTP(w, copy)
}

func (s *Server) verifyHandoffProof(ctx context.Context, account string, h store.Handoff) error {
	proof, err := s.DB.HandoffProof(ctx, account, h.ID)
	if err != nil {
		return err
	}
	if time.Since(proof.At) > 30*time.Minute || proof.Revision != h.Revision || !reflect.DeepEqual(proof.Destinations, h.Destinations) {
		return fmt.Errorf("test proof expired or changed")
	}
	for name, revision := range proof.Credentials {
		now, err := s.Vault.Revision(ctx, account, name)
		if err != nil || now != revision {
			return fmt.Errorf("credential changed after test")
		}
	}
	return nil
}

func (s *Server) testHandoff(w http.ResponseWriter, r *http.Request, t *Tenant, id string) {
	var input struct {
		OperationKey string `json:"operationKey"`
	}
	if decodeCloud(r, &input) != nil {
		cloudFail(w, 400, "invalid_test", "Provide one stable test operation key.")
		return
	}
	h, err := t.App.Handoff(r.Context(), id)
	if err != nil || h.Role != "target" || h.Phase != "prepared" {
		cloudFail(w, 409, "not_prepared", "Test a prepared target before source handoff.")
		return
	}
	manifest, err := t.App.Export(r.Context(), h.WatchID)
	if err != nil {
		cloudFail(w, 503, "watch_unavailable", "Could not read prepared definition.")
		return
	}
	bundle, err := plan.Parse([]byte(manifest))
	if err != nil || Policy(bundle) != nil {
		cloudFail(w, 409, "cloud_policy", "Prepared definition no longer satisfies cloud policy.")
		return
	}
	values, revisions := map[string]string{}, map[string]string{}
	add := func(ref *watch.SecretRef) error {
		if ref == nil {
			return nil
		}
		value, revision, err := s.Vault.GetWithRevision(r.Context(), t.Account.ID, ref.Env)
		if err != nil {
			return err
		}
		values[ref.Env], revisions[ref.Env] = value, revision
		return nil
	}
	for _, p := range bundle.Watches {
		for _, ref := range p.Definition.Spec.Source.Headers {
			if add(&ref) != nil {
				cloudFail(w, 409, "credential_required", "Rebind source credentials in this workspace.")
				return
			}
		}
	}
	for _, d := range bundle.Destinations {
		if add(d.Definition.Spec.URLRef) != nil {
			cloudFail(w, 409, "credential_required", "Rebind destination credentials in this workspace.")
			return
		}
		for _, ref := range d.Definition.Spec.Headers {
			if add(&ref) != nil {
				cloudFail(w, 409, "credential_required", "Rebind destination header credentials.")
				return
			}
		}
	}
	if len(bundle.Destinations) == 0 {
		cloudFail(w, 409, "destination_required", "Choose a remotely reachable alert destination.")
		return
	}
	digest, _ := watch.Revision(struct {
		ID, Manifest string
		Credentials  map[string]string
	}{id, manifest, revisions})
	probe, run, err := s.DB.BeginProbe(r.Context(), t.Account.ID, input.OperationKey, digest)
	if err != nil {
		cloudFail(w, 409, "test_conflict", "Keep the same operation key only for the same test and credential revision.")
		return
	}
	if !run {
		if probe.Outcome == "accepted" && s.verifyHandoffProof(r.Context(), t.Account.ID, h) != nil {
			cloudFail(w, 409, "test_expired", "Test proof expired or changed; explicitly send a new labeled test.")
			return
		}
		cloudWrite(w, 200, probe)
		return
	}
	lookup := func(name string) (string, bool) { v, ok := values[name]; return v, ok }
	fetch := source.HTTP{Client: t.App.HTTP.Client, Lookup: lookup}
	now := time.Now().UTC()
	batch := fetch.Fetch(r.Context(), bundle.Watches[0], "", now)
	accepted := len(batch.Observations) > 0
	for _, o := range batch.Observations {
		status, ok := o.Fields["http.status"].(int)
		if o.Health != "ok" || !ok || status < 200 || status >= 300 {
			accepted = false
		}
	}
	if accepted {
		for _, d := range bundle.Destinations {
			endpoint, err := source.Resolve(d.Definition.Spec.URLRef, lookup)
			if err != nil {
				accepted = false
				break
			}
			headers := http.Header{}
			for name, ref := range d.Definition.Spec.Headers {
				headers.Set(name, values[ref.Env])
			}
			payload, err := delivery.Render(d.Definition.Spec.Type, watch.Event{ID: input.OperationKey, WatchID: h.WatchID, Type: "test", At: now, Message: "Ding transfer test: confirming this destination before moving execution."})
			if err != nil {
				accepted = false
				break
			}
			result := delivery.HTTP(r.Context(), t.App.HTTP.Client, delivery.Request{URL: endpoint, Body: payload, Header: headers, Provider: d.Definition.Spec.Type}, now)
			if result.Outcome != delivery.Delivered {
				accepted = false
				break
			}
		}
	}
	outcome := "failed"
	if accepted {
		outcome = "accepted"
		err = s.DB.SaveHandoffProof(r.Context(), t.Account.ID, id, state.HandoffProof{Revision: h.Revision, Destinations: h.Destinations, Credentials: revisions, At: now})
	}
	if err != nil || s.DB.FinishProbe(r.Context(), t.Account.ID, input.OperationKey, outcome) != nil {
		cloudFail(w, 503, "outcome_unknown", "Inspect this same test operation; do not send a new key automatically.")
		return
	}
	probe.Outcome = outcome
	cloudWrite(w, 200, probe)
}
