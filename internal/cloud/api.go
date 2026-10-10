package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func decodeCloud(r *http.Request, value any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fmt.Errorf("trailing input")
	}
	return nil
}

func (s *Server) serveTenant(w http.ResponseWriter, r *http.Request, session state.Session, t *Tenant) {
	if r.Method == "GET" && r.URL.Path == "/v1/cloud/export" {
		s.accountExport(w, r, t)
		return
	}
	if r.Method == "DELETE" && r.URL.Path == "/v1/cloud/account" {
		s.accountDelete(w, r, session)
		return
	}
	if r.URL.Path == "/v1/handoffs" || strings.HasPrefix(r.URL.Path, "/v1/handoffs/") {
		s.cloudHandoff(w, r, t)
		return
	}
	if r.Method != "GET" && strings.HasPrefix(r.URL.Path, "/v1/cloud/secrets") {
		s.moveMu.Lock()
		defer s.moveMu.Unlock()
	}
	if r.URL.Path == "/v1/cloud/models" {
		s.modelConnections(w, r, t)
		return
	}
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/v1/info":
			cloudWrite(w, 200, map[string]any{"version": s.Version, "apiVersion": watch.APIVersion, "schema": store.SchemaVersion, "os": "cloud", "arch": "managed", "console": true, "origin": s.PublicURL, "listen": "", "stateDir": "", "limits": t.App.Limits, "time": time.Now().UTC(), "execution": "cloud", "workspace": session.Account})
			return
		case "/v1/cloud/usage":
			usage, err := s.DB.Usage(r.Context(), session.Account, time.Now())
			if err != nil {
				cloudFail(w, 503, "usage_unavailable", "Could not read workspace usage.")
				return
			}
			cloudWrite(w, 200, map[string]any{"usage": usage, "limits": map[string]any{"checks": state.MonthlyChecks, "deliveryAttempts": state.MonthlyDeliveries, "bytes": state.MonthlyBytes}, "note": "Failed checks and delivery attempts count. Storage pressure and pinned evidence are reported by Doctor."})
			return
		case "/v1/cloud/secrets":
			names, err := s.Vault.Names(r.Context(), session.Account)
			if err != nil {
				cloudFail(w, 503, "credentials_unavailable", "Could not read credential names.")
				return
			}
			cloudWrite(w, 200, map[string]any{"names": names})
			return
		}
	}
	if r.Method == "POST" && r.URL.Path == "/v1/cloud/secrets" {
		var req struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if decodeCloud(r, &req) != nil {
			cloudFail(w, 400, "invalid_credential", "Provide a credential name and value.")
			return
		}
		if err := s.Vault.Put(r.Context(), session.Account, req.Name, req.Value); err != nil {
			cloudFail(w, 400, "credential_rejected", "Credential name, size, or workspace limit is invalid.")
			return
		}
		cloudWrite(w, 200, map[string]string{"saved": req.Name})
		return
	}
	if r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/v1/cloud/secrets/") {
		name := strings.TrimPrefix(r.URL.Path, "/v1/cloud/secrets/")
		if err := s.Vault.Delete(r.Context(), session.Account, name); err != nil {
			cloudFail(w, 503, "delete_failed", "Could not delete the credential.")
			return
		}
		cloudWrite(w, 200, map[string]bool{"deleted": true})
		return
	}
	if r.Method == "POST" && r.URL.Path == "/v1/onboarding/preview" {
		s.cloudPreview(w, r, t)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/v1/cloud/destination-test" {
		s.testDestination(w, r, t)
		return
	}
	if !cloudRoute(r.Method, r.URL.Path) {
		cloudFail(w, 404, "not_found", "This operation is not part of the hosted API.")
		return
	}
	if r.Method == "POST" {
		// Bound and inspect manifests before forwarding even offline compile/test
		// requests. These cannot turn the hosted API into a command or jq runner.
		data, err := io.ReadAll(r.Body)
		if err != nil {
			cloudFail(w, 413, "request_limit", "Request exceeds the cloud limit.")
			return
		}
		var req struct {
			Manifest string `json:"manifest"`
		}
		if json.Unmarshal(data, &req) != nil {
			cloudFail(w, 400, "invalid_request", "Expected a JSON request.")
			return
		}
		if len(req.Manifest) > MaxManifestBytes {
			cloudFail(w, 413, "manifest_limit", "Cloud manifests must fit within 64 KiB.")
			return
		}
		if req.Manifest != "" {
			bundle, err := plan.Parse([]byte(req.Manifest))
			if err == nil {
				err = Policy(bundle)
			}
			if err != nil {
				cloudFail(w, 400, "cloud_policy", err.Error())
				return
			}
		}
		if r.URL.Path == "/v1/apply" {
			var apply watchrun.ApplyRequest
			if json.Unmarshal(data, &apply) != nil || (!apply.DryRun && apply.Review == nil) {
				cloudFail(w, 400, "review_required", "Preview this exact manifest before applying it.")
				return
			}
		}
		r = r.Clone(r.Context())
		r.Body = io.NopCloser(bytes.NewReader(data))
	}
	api := s.api(t)
	r = r.Clone(r.Context())
	r.Header = r.Header.Clone()
	r.Header.Set("Authorization", "Bearer "+api.token)
	api.handler.ServeHTTP(w, r)
}

func (s *Server) cloudPreview(w http.ResponseWriter, r *http.Request, t *Tenant) {
	var req FirstWatch
	if decodeCloud(r, &req) != nil {
		cloudFail(w, 400, "invalid_request", "Provide a watch ID, public URL, destination type, and credential reference.")
		return
	}
	manifest, err := Template(req)
	if err != nil {
		cloudFail(w, 400, "invalid_watch", err.Error())
		return
	}
	review, err := t.App.Apply(r.Context(), watchrun.ApplyRequest{Manifest: manifest, DryRun: true})
	if err != nil {
		cloudFail(w, 400, "preview_failed", err.Error())
		return
	}
	for _, change := range append(review.Changes, review.DestinationChanges...) {
		if change.State != "created" {
			cloudFail(w, 409, "already_exists", "Choose a new watch ID or edit this watch in Workbench.")
			return
		}
	}
	bundle, _ := plan.Parse([]byte(manifest))
	cloudWrite(w, 200, control.FirstWatchPreview{Manifest: manifest, Descriptions: []plan.Description{plan.Describe(bundle.Watches[0])}, Review: review})
}

func cloudRoute(method, path string) bool {
	if method == "GET" {
		switch path {
		case "/v1/status", "/v1/doctor", "/v1/watches", "/v1/events", "/v1/console/doctor", "/v1/console/watches", "/v1/console/events", "/v1/console/deliveries", "/v1/console/destinations":
			return true
		}
	}
	if method == "POST" {
		switch path {
		case "/v1/apply", "/v1/tools/compile", "/v1/tools/test", "/v1/tools/replay":
			return true
		}
	}
	p := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(p) < 3 || len(p) > 4 || p[0] != "v1" || p[2] == "" || p[2] == "." || p[2] == ".." {
		return false
	}
	if len(p) == 3 && method == "GET" {
		return p[1] == "watches" || p[1] == "events" || p[1] == "deliveries"
	}
	if len(p) != 4 {
		return false
	}
	return (method == "GET" && ((p[1] == "watches" && p[3] == "export") || (p[1] == "events" && p[3] == "observations"))) || (method == "POST" && ((p[1] == "watches" && p[3] == "lifecycle") || (p[1] == "deliveries" && p[3] == "retry")))
}
