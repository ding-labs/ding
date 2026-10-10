package cloud

import (
	"context"
	"net/http"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
)

func (s *Server) accountExport(w http.ResponseWriter, r *http.Request, t *Tenant) {
	records, err := t.App.List(r.Context())
	if err != nil {
		cloudFail(w, 503, "export_failed", "Could not read workspace definitions.")
		return
	}
	out := []map[string]string{}
	for _, record := range records {
		if record.Status == "deleted" {
			continue
		}
		id := record.Plan.Definition.Metadata.ID
		manifest, err := t.App.Export(r.Context(), id)
		if err != nil {
			cloudFail(w, 503, "export_failed", "A definition changed; retry export.")
			return
		}
		out = append(out, map[string]string{"id": id, "status": record.Status, "manifest": manifest})
	}
	cloudWrite(w, 200, map[string]any{"watches": out, "note": "Configuration export. Credential references require rebinding. Export does not stop watches, transfer history, or grant another runner ownership."})
}
func (s *Server) accountDelete(w http.ResponseWriter, r *http.Request, session state.Session) {
	var input struct {
		ConfirmWorkspace string `json:"confirmWorkspace"`
	}
	if decodeCloud(r, &input) != nil || input.ConfirmWorkspace != session.Account {
		cloudFail(w, 400, "confirmation_required", "Provide this exact workspace ID to permanently delete its active data and stop all watches.")
		return
	}
	if err := s.DB.BeginAccountDelete(r.Context(), session.Account); err != nil {
		cloudFail(w, 503, "delete_failed", "Could not begin account deletion.")
		return
	}
	// Request cancellation cannot resurrect an account after the durable fence.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	defer cancel()
	if err := s.Pool.Delete(cleanup, session.Account); err != nil {
		cloudFail(w, 503, "deletion_pending", "Access is revoked; deletion will finish during service recovery. Local copies remain independent.")
		return
	}
	if err := s.DB.FinishAccountDelete(cleanup, session.Account); err != nil {
		cloudFail(w, 503, "deletion_pending", "Execution stopped; control metadata deletion will finish during service recovery.")
		return
	}
	s.mu.Lock()
	delete(s.private, session.Account)
	delete(s.limits, session.Account)
	s.mu.Unlock()
	cookie(w, sessionCookie, "", time.Unix(1, 0))
	cookie(w, csrfCookie, "", time.Unix(1, 0))
	cloudWrite(w, 200, map[string]any{"deleted": true, "note": "Live workspace data and credentials removed. Encrypted backups are subject to the operator's disclosed retention. Local copies are not resumed or deleted."})
}

func (p *Pool) FinishPendingDeletes(ctx context.Context) error {
	ids, err := p.db.DeletingAccounts(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := p.Delete(ctx, id); err != nil {
			return err
		}
		if err := p.db.FinishAccountDelete(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
