package control

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/replay"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func integrationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrIntegrationDenied):
		fail(w, 403, "integration_denied", "grant expired, revoked, missing a required scope, or command/secret reference is not locally allowed")
	case errors.Is(err, store.ErrIntegrationLimit):
		fail(w, 429, "integration_limit", "integration storage limit reached; review local retention and outstanding previews")
	case errors.Is(err, store.ErrOperationConflict):
		fail(w, 409, "operation_conflict", err.Error())
	case errors.Is(err, store.ErrPreviewExpired):
		fail(w, 410, "preview_expired", err.Error())
	case errors.Is(err, store.ErrConflict):
		fail(w, 409, "revision_conflict", "state changed since review; inspect and preview again")
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrCursor), errors.Is(err, store.ErrCursorExpired), errors.Is(err, watchrun.ErrQuota), errors.Is(err, watchrun.ErrClosing):
		inspectionError(w, err)
	default:
		fail(w, 400, "integration_failed", err.Error())
	}
}

// Closed route set: integration tokens cannot mint browser sessions, back up the
// database, configure grants, or reach any other administrator API.
func integrationHandler(app *watchrun.App, c Credentials, base http.Handler) http.Handler {
	mux := http.NewServeMux()
	reader := http.NewServeMux()
	reader.Handle("/", base)
	consoleRoutes(reader, app, ConsoleConfig{})
	access := func(w http.ResponseWriter, r *http.Request, scope string) (store.IntegrationGrant, bool) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			integrationError(w, store.ErrIntegrationDenied)
			return store.IntegrationGrant{}, false
		}
		g, err := app.IntegrationAccess(r.Context(), token, scope)
		if err != nil {
			integrationError(w, err)
			return g, false
		}
		return g, true
	}
	admin := func(w http.ResponseWriter, r *http.Request) bool {
		if !bearer(r, c.Admin) {
			fail(w, 403, "bearer_required", "local administrator pairing required")
			return false
		}
		return true
	}
	mux.HandleFunc("POST /v1/integrations/grants", func(w http.ResponseWriter, r *http.Request) {
		if !admin(w, r) {
			return
		}
		var req watchrun.GrantRequest
		if decodeBounded(w, r, &req, 16384) != nil {
			fail(w, 400, "invalid_request", "invalid integration grant")
			return
		}
		out, err := app.CreateIntegrationGrant(r.Context(), req)
		if err != nil {
			integrationError(w, err)
			return
		}
		write(w, 201, out, nil)
	})
	mux.HandleFunc("GET /v1/integrations/grants", func(w http.ResponseWriter, r *http.Request) {
		if !admin(w, r) {
			return
		}
		var out []store.IntegrationGrant
		err := app.Store.View(r.Context(), func(tx *store.Tx) error { var err error; out, err = tx.IntegrationGrants(); return err })
		if err != nil {
			integrationError(w, err)
			return
		}
		write(w, 200, out, nil)
	})
	mux.HandleFunc("DELETE /v1/integrations/grants/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !admin(w, r) {
			return
		}
		err := app.Store.Update(r.Context(), func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(r.PathValue("id")) })
		if err != nil {
			integrationError(w, err)
			return
		}
		write(w, 200, map[string]bool{"revoked": true}, nil)
	})
	mux.HandleFunc("GET /v1/integrations/capabilities", func(w http.ResponseWriter, r *http.Request) {
		g, ok := access(w, r, "inspect")
		if !ok {
			return
		}
		var instance string
		err := app.Store.View(r.Context(), func(tx *store.Tx) error { var err error; instance, err = tx.Identity(); return err })
		if err != nil {
			integrationError(w, err)
			return
		}
		write(w, 200, map[string]any{"version": watchrun.IntegrationVersion, "instance": instance, "grant": g, "previewTTLSeconds": 900, "maxPageSize": 100, "maxManifestBytes": 1 << 20, "maxFixtureBytes": 8 << 20, "operationReceipts": "durable; no automatic expiration; maximum 10000", "eventsSubscriptions": false}, nil)
	})
	for _, kind := range []string{"watches", "events", "deliveries", "destinations"} {
		mux.HandleFunc("GET /v1/integrations/"+kind, func(w http.ResponseWriter, r *http.Request) {
			if _, ok := access(w, r, "inspect"); !ok {
				return
			}
			forwarded := r.Clone(r.Context())
			forwarded.URL.Path = "/v1/console/" + kind
			reader.ServeHTTP(w, forwarded)
		})
	}
	for _, kind := range []string{"watches", "events", "deliveries"} {
		mux.HandleFunc("GET /v1/integrations/"+kind+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := access(w, r, "inspect"); !ok {
				return
			}
			forwarded := r.Clone(r.Context())
			id := r.PathValue("id")
			if id == "." || id == ".." || strings.ContainsAny(id, "/\\\x00") {
				fail(w, 400, "invalid_request", "invalid record ID")
				return
			}
			forwarded.URL.Path = "/v1/" + kind + "/" + id
			forwarded.URL.RawPath = ""
			reader.ServeHTTP(w, forwarded)
		})
	}
	slots := make(chan struct{}, 2)
	mux.HandleFunc("POST /v1/integrations/preview", func(w http.ResponseWriter, r *http.Request) {
		g, ok := access(w, r, "preview")
		if !ok {
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			fail(w, 429, "tool_busy", "two previews are already running")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		var req ToolRequest
		if decodeBounded(w, r, &req, 12<<20) != nil || len(req.Manifest) > 1<<20 || len(req.Fixture) > 8<<20 || len(req.Evidence) > 0 {
			fail(w, 400, "invalid_request", "preview requires a manifest up to 1 MiB and optional fixture up to 8 MiB")
			return
		}
		compiled := compileTool(app, req.Manifest)
		if !compiled.Valid {
			write(w, 200, map[string]any{"valid": false, "diagnostics": compiled.Diagnostics}, nil)
			return
		}
		var fixture any
		if req.Fixture != "" {
			var selected *plan.Compiled
			for i := range compiled.Bundle.Watches {
				p := &compiled.Bundle.Watches[i]
				if p.Definition.Metadata.ID == req.Watch || (req.Watch == "" && len(compiled.Bundle.Watches) == 1) {
					selected = p
				}
			}
			if selected == nil {
				fail(w, 400, "invalid_watch", "select one watch for the fixture")
				return
			}
			report, err := replay.RunContext(ctx, *selected, strings.NewReader(req.Fixture))
			if err != nil {
				fail(w, 400, "invalid_fixture", err.Error())
				return
			}
			fixture = map[string]any{"watchId": report.WatchID, "revision": report.Revision, "observations": report.Observations, "eventCount": len(report.Events), "events": report.Events[:min(20, len(report.Events))], "truncated": len(report.Events) > 20}
		}
		out, err := app.PreviewIntegration(ctx, g.ID, req.Manifest)
		if err != nil {
			integrationError(w, err)
			return
		}
		write(w, 200, map[string]any{"valid": true, "preview": out, "descriptions": compiled.Descriptions, "fixture": fixture}, nil)
	})
	mux.HandleFunc("POST /v1/integrations/apply", func(w http.ResponseWriter, r *http.Request) {
		g, ok := access(w, r, "manage")
		if !ok {
			return
		}
		var req struct {
			Handle       string `json:"handle"`
			OperationKey string `json:"operationKey"`
		}
		if decodeBounded(w, r, &req, 4096) != nil || len(req.Handle) != 64 {
			fail(w, 400, "invalid_request", "preview handle and operation key required")
			return
		}
		out, err := app.ApplyIntegration(r.Context(), g.ID, req.Handle, req.OperationKey)
		if err != nil {
			integrationError(w, err)
			return
		}
		write(w, 200, out, nil)
	})
	mux.HandleFunc("POST /v1/integrations/watches/{id}/lifecycle", func(w http.ResponseWriter, r *http.Request) {
		g, ok := access(w, r, "manage")
		if !ok {
			return
		}
		var req struct {
			watchrun.LifecycleRequest
			OperationKey string `json:"operationKey"`
		}
		if decodeBounded(w, r, &req, 4096) != nil {
			fail(w, 400, "invalid_request", "invalid lifecycle request")
			return
		}
		out, err := app.LifecycleIntegration(r.Context(), g.ID, r.PathValue("id"), req.OperationKey, req.LifecycleRequest)
		if err != nil {
			integrationError(w, err)
			return
		}
		write(w, 200, out, nil)
	})
	mux.HandleFunc("POST /v1/integrations/deliveries/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		g, ok := access(w, r, "retry")
		if !ok {
			return
		}
		var req struct {
			OperationKey string `json:"operationKey"`
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 || decodeBounded(w, r, &req, 4096) != nil {
			fail(w, 400, "invalid_request", "delivery ID and operation key required")
			return
		}
		if err = app.RetryIntegration(r.Context(), g.ID, id, req.OperationKey); err != nil {
			integrationError(w, err)
			return
		}
		write(w, 200, map[string]any{"id": id, "status": "pending"}, nil)
	})
	mux.HandleFunc("GET /v1/integrations/operations/{key}", func(w http.ResponseWriter, r *http.Request) {
		g, ok := access(w, r, "inspect")
		if !ok {
			return
		}
		out, err := app.GetIntegrationOperation(r.Context(), g.ID, r.PathValue("key"))
		if err != nil {
			integrationError(w, err)
			return
		}
		write(w, 200, out, nil)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Header.Get("Origin") != "" {
			fail(w, 403, "invalid_origin", "integration endpoints require a native MCP adapter")
			return
		}
		mux.ServeHTTP(w, r)
	})
}
