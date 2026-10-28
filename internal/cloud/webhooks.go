package cloud

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/database"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/transform"
	stripehook "github.com/stripe/stripe-go/v86/webhook"
)

type verifiedPushKey struct{}
type verifiedPush struct {
	guard      database.TransactionGuard
	admitted   bool
	validUntil time.Time
	grant      state.Service
	digest     string
	bytes      int
	authorize  func(context.Context) error
}

func (s *Server) webhooksAPI(w http.ResponseWriter, r *http.Request, p access.Principal) bool {
	if r.URL.Path != "/v1/webhooks" && !strings.HasPrefix(r.URL.Path, "/v1/webhooks/") {
		return false
	}
	if !s.Push.NativeWebhooks {
		cloudFail(w, 404, "unavailable", "Native webhooks are not enabled on this instance.")
		return true
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/webhooks/"), "/")
	switch {
	case r.Method == "GET" && r.URL.Path == "/v1/webhooks":
		out, err := s.DB.Webhooks(r.Context(), p.Workspace)
		if err != nil {
			cloudFail(w, 503, "connections_unavailable", "Could not read connections.")
		} else {
			cloudWrite(w, 200, out)
		}
	case r.Method == "POST" && r.URL.Path == "/v1/webhooks":
		var input struct {
			Name         string   `json:"name"`
			Provider     string   `json:"provider"`
			Watch        string   `json:"watch"`
			Secret       string   `json:"secret"`
			Repositories []string `json:"repositories"`
			Account      string   `json:"account"`
			Live         bool     `json:"live"`
			Events       []string `json:"events"`
		}
		if decodeCloud(r, &input) != nil {
			cloudFail(w, 400, "invalid_request", "Provide connection settings and a private signing secret.")
			return true
		}
		lifetimes, err := s.producerLifetimes(r.Context(), p.Workspace, []string{input.Watch})
		if err != nil {
			cloudFail(w, 400, "invalid_watch", "Select an existing push watch.")
			return true
		}
		out, err := s.Vault.CreateWebhook(r.Context(), p, state.WebhookConnection{Name: input.Name, Provider: input.Provider, Watch: input.Watch, Lifetime: lifetimes[input.Watch], Repositories: input.Repositories, Account: input.Account, Live: input.Live, Events: input.Events}, input.Secret)
		if err != nil {
			cloudFail(w, 400, "connection_denied", err.Error())
		} else {
			cloudWrite(w, 201, map[string]any{"connection": out, "url": s.PublicURL + "/v1/hooks/" + out.Provider + "/" + out.ID})
		}
	case r.Method == "POST" && len(parts) == 2 && parts[1] == "rotate":
		var input struct {
			Secret         string `json:"secret"`
			OverlapSeconds int    `json:"overlapSeconds"`
		}
		if decodeCloud(r, &input) != nil {
			cloudFail(w, 400, "invalid_request", "Provide signing secret and overlap seconds.")
			return true
		}
		if err := s.Vault.RotateWebhook(r.Context(), p, parts[0], input.Secret, input.OverlapSeconds); err != nil {
			cloudFail(w, 400, "rotation_denied", err.Error())
		} else {
			cloudWrite(w, 200, map[string]bool{"rotated": true})
		}
	case r.Method == "DELETE" && len(parts) == 1:
		if err := s.DB.DeleteWebhook(r.Context(), p, parts[0]); err != nil {
			cloudFail(w, 403, "delete_denied", err.Error())
		} else {
			cloudWrite(w, 200, map[string]bool{"deleted": true})
		}
	default:
		cloudFail(w, 405, "method_not_allowed", "Unsupported connection operation.")
	}
	return true
}

func (s *Server) serveWebhook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", "1")
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/hooks/"), "/")
	if r.Method != "POST" || len(parts) != 2 || !accountID.MatchString(parts[1]) || r.URL.RawPath != "" {
		cloudFail(w, 404, "not_found", "Webhook connection unavailable.")
		return
	}
	c, err := s.Vault.Webhook(r.Context(), parts[1])
	if err != nil || c.Provider != parts[0] {
		cloudFail(w, 404, "not_found", "Webhook connection unavailable.")
		return
	}
	releaseBody, ok := s.admitWebhookBody(c.Workspace)
	if !ok {
		cloudFail(w, 429, "hook_capacity", "Webhook connection capacity reached.")
		return
	}
	defer releaseBody()
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(4 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(s.Pool.Capacity.Effective().MaxBodyBytes)))
	if err != nil {
		cloudFail(w, 413, "input_limit", "Webhook body unavailable or exceeds limit.")
		return
	}
	if r.Header.Get("Content-Encoding") != "" {
		cloudFail(w, 415, "unsupported_media", "Send an uncompressed webhook.")
		return
	}
	secret := ""
	for _, candidate := range []string{c.Secret, c.PreviousSecret} {
		if candidate != "" && validSignature(c.Provider, r.Header, raw, candidate) {
			secret = candidate
			break
		}
	}
	if secret == "" {
		cloudFail(w, 401, "signature_invalid", "Webhook signature is invalid or expired.")
		return
	}
	if err := transform.ValidateJSON(ctx, raw, s.Push.effective().MaxDepth, s.Push.effective().MaxNodes); err != nil {
		cloudFail(w, 400, "invalid_payload", "Webhook JSON is invalid or exceeds structural bounds.")
		return
	}
	release, ok := s.admitPush(c.Workspace, "hook:"+c.ID, len(raw))
	if !ok {
		cloudFail(w, 429, "ingest_rate", "Workspace webhook capacity reached.")
		return
	}
	defer release()
	body, key, ignored, err := webhookInput(c, r.Header, raw)
	if err != nil {
		cloudFail(w, 400, "unsupported_event", err.Error())
		return
	}
	var trusted *verifiedPush
	authorize := func(ctx context.Context) error {
		current, err := s.Vault.Webhook(ctx, c.ID)
		if err != nil {
			return access.ErrDenied
		}
		if current.Workspace != c.Workspace || current.Watch != c.Watch || current.Lifetime != c.Lifetime {
			return access.ErrDenied
		}
		if subtle.ConstantTimeCompare([]byte(current.Secret), []byte(secret)) != 1 && subtle.ConstantTimeCompare([]byte(current.PreviousSecret), []byte(secret)) != 1 {
			return access.ErrDenied
		}
		if trusted != nil {
			trusted.validUntil = time.Time{}
			if subtle.ConstantTimeCompare([]byte(current.Secret), []byte(secret)) != 1 {
				trusted.validUntil = current.PreviousUntil
			}
		}
		return nil
	}
	if ignored {
		s.DB.Access.RLock()
		defer s.DB.Access.RUnlock()
		if err := authorize(ctx); err != nil {
			cloudFail(w, 401, "connection_revoked", "Webhook connection changed.")
			return
		}
		cloudWrite(w, 200, map[string]any{"ignored": true, "observations": 0})
		return
	}
	digest := sha256.Sum256(raw)
	trusted = &verifiedPush{admitted: true, grant: state.Service{ID: "hook:" + c.ID, Workspace: c.Workspace, Scopes: []string{"ingest"}, Watches: []string{c.Watch}, Lifetimes: map[string]string{c.Watch: c.Lifetime}}, digest: hex.EncodeToString(digest[:]), bytes: len(raw), authorize: authorize}
	if subtle.ConstantTimeCompare([]byte(c.Secret), []byte(secret)) != 1 {
		trusted.validUntil = c.PreviousUntil
	}
	trusted.guard = s.Vault.WebhookIngestGuard(c.ID, c.Workspace, authorize, func() time.Time { return trusted.validUntil })
	copy := r.Clone(context.WithValue(ctx, verifiedPushKey{}, trusted))
	copy.URL.Path = "/v1/ingest/" + c.Watch
	copy.URL.RawPath = ""
	copy.Body = io.NopCloser(bytes.NewReader(body))
	copy.ContentLength = int64(len(body))
	copy.Header = make(http.Header)
	copy.Header.Set("Content-Type", "application/json")
	copy.Header.Set("Idempotency-Key", key)
	s.measuredPush(w, copy)
}

func validSignature(provider string, header http.Header, raw []byte, secret string) bool {
	switch provider {
	case "github":
		if len(header.Values("X-Hub-Signature-256")) != 1 {
			return false
		}
		signature, ok := strings.CutPrefix(header.Get("X-Hub-Signature-256"), "sha256=")
		if !ok {
			return false
		}
		want, err := hex.DecodeString(signature)
		if err != nil {
			return false
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(raw)
		return hmac.Equal(want, mac.Sum(nil))
	case "stripe":
		return len(header.Values("Stripe-Signature")) == 1 && stripehook.ValidatePayload(raw, header.Get("Stripe-Signature"), secret) == nil
	case "workos":
		return validWorkOSSignature(header, raw, secret, time.Now())
	}
	return false
}

func webhookInput(c state.WebhookConnection, header http.Header, raw []byte) ([]byte, string, bool, error) {
	if c.Provider == "workos" {
		return workOSSignupInput(raw)
	}
	if c.Provider == "github" {
		var event struct {
			Action     string `json:"action"`
			Zen        string `json:"zen"`
			Repository struct {
				ID       json.Number `json:"id"`
				FullName string      `json:"full_name"`
			} `json:"repository"`
			Run struct {
				ID         json.Number `json:"id"`
				Attempt    int         `json:"run_attempt"`
				Name       string      `json:"name"`
				Status     string      `json:"status"`
				Conclusion string      `json:"conclusion"`
			} `json:"workflow_run"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, "", false, fmt.Errorf("invalid GitHub event")
		}
		if !slices.Contains(c.Repositories, event.Repository.ID.String()) {
			return nil, "", false, fmt.Errorf("repository is not permitted")
		}
		switch header.Get("X-GitHub-Event") {
		case "ping":
			if event.Zen != "" {
				return nil, "", true, nil
			}
		case "workflow_run":
			if event.Run.ID == "" || event.Run.Name == "" {
				return nil, "", false, fmt.Errorf("missing workflow identity")
			}
			if event.Action != "completed" || event.Run.Status != "completed" {
				return nil, "", true, nil
			}
			if !slices.Contains([]string{"success", "failure", "timed_out", "action_required", "startup_failure"}, event.Run.Conclusion) {
				return nil, "", true, nil
			}
			key := header.Get("X-GitHub-Delivery")
			if key == "" || len(key) > 256 || len(header.Values("X-GitHub-Delivery")) != 1 {
				return nil, "", false, fmt.Errorf("delivery ID required")
			}
			body, err := json.Marshal(map[string]any{"repo": event.Repository.FullName, "repositoryId": event.Repository.ID.String(), "workflow": event.Run.Name, "runId": event.Run.ID.String(), "attempt": event.Run.Attempt, "eventId": event.Repository.ID.String() + ":" + event.Run.ID.String() + ":" + fmt.Sprint(event.Run.Attempt), "conclusion": event.Run.Conclusion, "failed": event.Run.Conclusion != "success"})
			return body, key, false, err
		}
		return nil, "", false, fmt.Errorf("only GitHub workflow_run and repository ping events are supported")
	}
	var event struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Type    string `json:"type"`
		Account string `json:"account"`
		Live    *bool  `json:"livemode"`
		Data    struct {
			Object map[string]any `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Object != "event" || event.ID == "" || event.Live == nil || event.Data.Object == nil {
		return nil, "", false, fmt.Errorf("Stripe snapshot event required; thin events are unsupported")
	}
	if *event.Live != c.Live || (event.Account != "" && event.Account != c.Account) {
		return nil, "", false, fmt.Errorf("Stripe account or mode does not match connection")
	}
	if !slices.Contains(c.Events, event.Type) {
		return nil, "", true, nil
	}
	return raw, event.ID, false, nil
}
