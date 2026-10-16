package cloud

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/database"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/transform"
	"github.com/ding-labs/ding/internal/watchrun"
)

func (s *Server) pushCapabilities(workspace string) map[string]any {
	if s.Shared {
		return map[string]any{"supported": true, "admissionOpen": true, "idempotencyRequired": false, "projectionModes": []string{"fields", "itemsPath", "jq"}, "automaticTransfer": false, "contract": "shared-ingest"}
	}
	p := s.Push.effective()
	enabled := !p.Disabled && (len(p.Workspaces) == 0 || slices.Contains(p.Workspaces, workspace))
	return map[string]any{"supported": true, "admissionOpen": enabled, "maxBodyBytes": s.Pool.Capacity.Effective().MaxBodyBytes, "maxDepth": p.MaxDepth, "maxNodes": p.MaxNodes, "receiptSeconds": p.ReceiptSeconds, "idempotencyRequired": true, "projectionModes": []string{"fields", "itemsPath"}, "automaticTransfer": false}
}

func (s *Server) servePush(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", "1")
	if r.Method != "POST" {
		cloudFail(w, 405, "method_not_allowed", "Use POST for ingestion.")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/ingest/")
	if r.URL.RawQuery != "" || id == "" || len(id) > 128 || strings.ContainsAny(id, "/%\\") || id == "." || id == ".." || (r.URL.RawPath != "" && r.URL.RawPath != r.URL.Path) || len(r.Header.Values("Authorization")) > 1 {
		cloudFail(w, 400, "invalid_request", "Use an unambiguous watch path and one authorization header.")
		return
	}
	verified, _ := r.Context().Value(verifiedPushKey{}).(*verifiedPush)
	var token string
	var grant state.Service
	var err error
	if verified != nil {
		grant = verified.grant
	} else {
		var ok bool
		token, ok = strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !strings.HasPrefix(token, "ding_service_") || len(token) != len("ding_service_")+64 {
			cloudFail(w, 401, "producer_required", "Use a scoped producer credential.")
			return
		}
		grant, err = s.DB.ServiceToken(r.Context(), token)
		if err != nil {
			pushAuthorizationError(w, err)
			return
		}
	}
	if !slices.Contains(grant.Scopes, "ingest") || !slices.Contains(grant.Watches, id) || (r.Header.Get("X-Ding-Workspace") != "" && r.Header.Get("X-Ding-Workspace") != grant.Workspace) {
		cloudFail(w, 403, "watch_denied", "Producer does not permit this watch.")
		return
	}
	if tracked, ok := w.(*pushResponse); ok {
		tracked.workspace = grant.Workspace
		tracked.watch = id
		tracked.lifetime = grant.Lifetimes[id]
	}
	p := s.Push.effective()
	cap := s.Pool.Capacity.Effective()
	cost := cap.MaxBodyBytes
	if r.ContentLength >= 0 && r.ContentLength < int64(cost) {
		cost = int(r.ContentLength)
	}
	release := func() {}
	if verified == nil || !verified.admitted {
		var ok bool
		release, ok = s.admitPush(grant.Workspace, grant.ID, cost)
		if !ok {
			cloudFail(w, 429, "ingest_rate", "Producer or workspace capacity reached; retry identical input later.")
			return
		}
	}
	defer release()
	key := r.Header.Get("Idempotency-Key")
	if len(r.Header.Values("Idempotency-Key")) != 1 || len(key) == 0 || len(key) > 256 || strings.IndexFunc(key, func(c rune) bool { return c < 33 || c > 126 }) >= 0 {
		cloudFail(w, 400, "invalid_key", "Supply one Idempotency-Key of 1–256 printable ASCII characters without spaces.")
		return
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) || r.Header.Get("Content-Encoding") != "" {
		cloudFail(w, 415, "unsupported_media", "Send uncompressed UTF-8 application/json.")
		return
	}
	t, err := s.Pool.Get(r.Context(), grant.Workspace)
	if err != nil {
		cloudFail(w, 503, "workspace_unavailable", "Workspace unavailable.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	stop := context.AfterFunc(t.ctx, cancel)
	defer stop()
	principal := access.Principal{User: "service:" + grant.ID, Workspace: grant.Workspace, Role: access.Editor, Kind: "service", Grant: grant.ID, Scopes: grant.Scopes}
	ctx = access.WithOperation(access.WithPrincipal(ctx, principal), r.Method, r.URL.Path)
	record, err := t.App.Record(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		cloudFail(w, 404, "not_found", "Watch unavailable.")
		return
	}
	if err != nil {
		cloudFail(w, 503, "store_unavailable", "Cannot read watch.")
		return
	}
	if record.Status == "deleted" || record.Lifetime == "" || grant.Lifetimes[id] != record.Lifetime {
		cloudFail(w, 403, "watch_lifetime", "Reauthorize this producer for the current watch.")
		return
	}
	if tracked, ok := w.(*pushResponse); ok {
		tracked.workspace = grant.Workspace
		tracked.watch = id
		tracked.lifetime = record.Lifetime
	}
	// Read deadlines bound slow bodies without holding the authorization fence.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(time.Duration(p.ReadSeconds) * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(cap.MaxBodyBytes)))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			cloudFail(w, 413, "input_limit", "Body exceeds cloud input limit.")
		} else {
			cloudFail(w, 400, "body_unavailable", "Body was incomplete or timed out.")
		}
		return
	}
	input := watchrun.NewPushInput(grant.ID, key, raw, p.MaxReceipts, time.Duration(p.ReceiptSeconds)*time.Second)
	input.ValidUntil = grant.ExpiresAt
	if verified != nil {
		input.ValidUntil = verified.validUntil
		input.Digest = verified.digest
		input.Bytes = verified.bytes
	}
	authorize := func() error {
		if verified != nil {
			err := verified.authorize(ctx)
			input.ValidUntil = verified.validUntil
			return err
		}
		current, err := s.DB.ServiceToken(ctx, token)
		if err != nil {
			return err
		}
		input.ValidUntil = current.ExpiresAt
		if current.ID != grant.ID || current.Workspace != grant.Workspace || current.Lifetimes[id] != record.Lifetime || !slices.Contains(current.Scopes, "ingest") {
			return access.ErrDenied
		}
		return nil
	}
	s.DB.Access.RLock()
	err = authorize()
	if err != nil {
		s.DB.Access.RUnlock()
		pushAuthorizationError(w, err)
		return
	}
	var receipt store.PushReceipt
	if err == nil {
		receipt, err = t.App.ReplayPush(ctx, record, input)
	}
	s.DB.Access.RUnlock()
	if err == nil {
		if receipt.Duplicate {
			s.Pool.ingest.duplicates.Add(1)
		}
		s.DB.NoteServiceUse(ctx, grant)
		cloudWrite(w, 202, receipt)
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		s.pushError(w, err)
		return
	}
	if p.Disabled || (len(p.Workspaces) > 0 && !slices.Contains(p.Workspaces, grant.Workspace)) {
		cloudFail(w, 503, "ingest_closed", "New input admission is temporarily closed; retain this request for retry.")
		return
	}
	if available, err := freeDiskBytes(s.Pool.root); err != nil || available < uint64(p.MinFreeBytes) {
		cloudFail(w, 503, "disk_reserve", "Host storage reserve is low; retain this input for retry.")
		return
	}
	if len(raw) > record.Plan.Definition.Spec.Limits.MaxBytes {
		cloudFail(w, 413, "input_limit", "Body exceeds watch input limit.")
		return
	}
	parseStart := time.Now()
	batch, err := watchrun.PreparePush(ctx, record, raw, p.MaxDepth, p.MaxNodes)
	s.Pool.ingest.parse.observe(time.Since(parseStart))
	if err != nil {
		s.pushError(w, err)
		return
	}
	s.DB.Access.RLock()
	defer s.DB.Access.RUnlock()
	if err = authorize(); err != nil {
		pushAuthorizationError(w, err)
		return
	}
	guard := s.DB.IngestGuard(token, grant.ID, grant.Workspace, id, record.Lifetime)
	if verified != nil {
		guard = verified.guard
	}
	ctx = database.WithTransactionGuard(ctx, guard)
	commitStart := time.Now()
	receipt, err = t.App.CommitPush(ctx, record, batch, input)
	s.Pool.ingest.commit.observe(time.Since(commitStart))
	if err != nil {
		s.pushError(w, err)
		return
	}
	if receipt.Duplicate {
		s.Pool.ingest.duplicates.Add(1)
	}
	s.DB.NoteServiceUse(ctx, grant)
	cloudWrite(w, 202, receipt)
}

func pushAuthorizationError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, access.ErrDenied) || errors.Is(err, state.ErrServiceRevoked) {
		cloudFail(w, 401, "service_revoked", "Producer credential expired or unavailable.")
		return
	}
	cloudFail(w, 503, "authorization_unavailable", "Cannot verify producer access; retry identical input with the same key.")
}

func (s *Server) pushError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, watchrun.ErrProducerExpired), errors.Is(err, state.ErrServiceRevoked):
		cloudFail(w, 401, "service_revoked", "Producer credential expired before acceptance.")
	case errors.Is(err, store.ErrIdempotency):
		cloudFail(w, 409, "idempotency_conflict", "This key already accepted different bytes.")
	case errors.Is(err, store.ErrStale):
		cloudFail(w, 409, "inactive_source", "Watch changed or is not an active push source.")
	case errors.Is(err, transform.ErrLimit):
		cloudFail(w, 413, "input_limit", "Projected input exceeds configured limits.")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		cloudFail(w, 503, "acceptance_uncertain", "Acceptance could not be confirmed; retry identical bytes with the same key.")
	case errors.Is(err, watchrun.ErrInput):
		cloudFail(w, 400, "invalid_input", err.Error())
	case errors.Is(err, watchrun.ErrQuota):
		cloudFail(w, 429, "quota_exceeded", "Workspace storage or pending delivery capacity reached.")
	case errors.Is(err, access.ErrDenied):
		cloudFail(w, 403, "watch_denied", "Producer access changed.")
	default:
		cloudFail(w, 503, "acceptance_uncertain", "Acceptance could not be confirmed; retry identical bytes with the same key.")
	}
}

// Require a live push source and bind the grant to its immutable lifetime.
func (s *Server) producerLifetimes(ctx context.Context, workspace string, ids []string) (map[string]string, error) {
	t, err := s.Pool.Get(ctx, workspace)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, id := range ids {
		r, err := t.App.Record(ctx, id)
		if err != nil {
			return nil, err
		}
		if r.Status == "deleted" || r.Plan.Definition.Spec.Source.Type != "push" {
			return nil, watchrun.ErrInput
		}
		out[id] = r.Lifetime
	}
	return out, nil
}
