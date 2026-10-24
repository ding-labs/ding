package cloud

import (
	"net/http"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/store"
)

func (s *Server) scopedMCP(t *Tenant, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/integrations/transfers/step" {
			p, ok := access.FromContext(r.Context())
			if !ok || !p.Allows(access.Move) {
				cloudFail(w, 403, "integration_denied", "Owner-approved move permission is required.")
				return
			}
			if _, err := t.App.IntegrationAccess(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "move"); err != nil {
				cloudFail(w, 403, "integration_denied", "Move consent expired or was revoked.")
				return
			}
			control.TransferStep(w, r, t.App, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.cloudHandoff(w, r, t) }))
			return
		}
		if r.Method != "GET" || r.URL.Path != "/v1/integrations/cloud/status" {
			next.ServeHTTP(w, r)
			return
		}
		p, ok := access.FromContext(r.Context())
		if !ok || !p.Allows(access.Inspect) {
			cloudFail(w, 403, "integration_denied", "Inspection permission is required.")
			return
		}
		if _, err := t.App.IntegrationAccess(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "inspect"); err != nil {
			cloudFail(w, 403, "integration_denied", "Model consent expired or was revoked.")
			return
		}
		if s.Shared {
			cloudFail(w, 400, "unsupported_operation", "This instance is self-hosted; inspect its capabilities and watches.")
			return
		}
		workspace, err := s.DB.Workspace(r.Context(), t.Workspace.ID)
		if err != nil {
			cloudFail(w, 503, "workspace_unavailable", "Workspace settings are unavailable.")
			return
		}
		profile := t.App.Integration
		profile.WorkspaceName = workspace.Name
		now := time.Now().UTC()
		usage, err := s.DB.Usage(r.Context(), t.Workspace.ID, now)
		if err != nil {
			cloudFail(w, 503, "usage_unavailable", "Workspace usage is unavailable.")
			return
		}
		health, err := t.App.Doctor(r.Context())
		if err != nil {
			cloudFail(w, 503, "health_unavailable", "Workspace health is unavailable.")
			return
		}
		capacity := s.Pool.Capacity.Effective()
		var inbound store.PushUsage
		if err := t.App.Store.View(r.Context(), func(tx *store.Tx) error { var e error; inbound, e = tx.PushUsage(); return e }); err != nil {
			cloudFail(w, 503, "usage_unavailable", "Workspace usage is unavailable.")
			return
		}
		cloudWrite(w, 200, map[string]any{
			"ingest": inbound, "push": s.pushCapabilities(t.Workspace.ID),
			"usage": usage, "limits": map[string]any{"checks": capacity.OutboundChecks, "deliveryAttempts": capacity.OutboundDeliveries, "bytes": capacity.OutboundBytes},
			"resetsAt":        time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC),
			"budgetExhausted": (capacity.OutboundChecks > 0 && usage.Checks >= capacity.OutboundChecks) || (capacity.OutboundDeliveries > 0 && usage.Deliveries >= capacity.OutboundDeliveries) || (capacity.OutboundBytes > 0 && usage.Bytes >= capacity.OutboundBytes),
			"health":          map[string]any{"healthy": health.Healthy, "running": health.Running, "closing": health.Closing, "storage": health.Usage, "storageLimits": health.Limits, "deliveries": health.Deliveries},
			"policy":          map[string]any{"execution": "cloud", "minIntervalSeconds": capacity.MinPollSeconds, "publicHTTPOnly": false, "httpRequiresPublicURL": true, "sourceTypes": []string{"http", "push"}, "destinations": []string{"webhook", "slack", "discord"}, "preview": profile},
			"note":            "Failed checks and delivery attempts count. Zero cumulative limits mean no monthly quota. Exhausted configured budgets block outbound work; evidence pinned by active incidents or pending deliveries counts against storage limits.",
		})
	})
}
