package cloud

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/store"
)

// Recent outcomes contain no body, token, producer-supplied key, or error text.
// They are bounded operational diagnostics, not an acceptance ledger.
type pushOutcome struct {
	Lifetime  string    `json:"-"`
	RequestID string    `json:"requestId"`
	Watch     string    `json:"watchId"`
	At        time.Time `json:"at"`
	Status    int       `json:"status"`
	Code      string    `json:"code"`
}

func (s *Server) recordPushOutcome(workspace string, out pushOutcome) {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	if s.pushOutcomes == nil {
		s.pushOutcomes = map[string][]pushOutcome{}
	}
	if _, ok := s.pushOutcomes[workspace]; !ok && len(s.pushOutcomes) >= MaxAccounts {
		return
	}
	rows := s.pushOutcomes[workspace]
	if len(rows) >= 100 {
		copy(rows, rows[len(rows)-99:])
		rows = rows[:99]
	}
	s.pushOutcomes[workspace] = append(rows, out)
}
func (s *Server) servePushStatus(w http.ResponseWriter, r *http.Request, t *Tenant) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/ingest-status/")
	record, err := t.App.Record(r.Context(), id)
	if err != nil || record.Status == "deleted" {
		cloudFail(w, 404, "not_found", "Watch unavailable.")
		return
	}
	var last *store.PushReceipt
	err = t.App.Store.View(r.Context(), func(tx *store.Tx) error {
		receipt, e := tx.LastPush(record.Lifetime, id)
		if e == nil {
			last = &receipt
		}
		return e
	})
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		cloudFail(w, 503, "store_unavailable", "Cannot read acceptance status.")
		return
	}
	s.pushMu.Lock()
	outcomes := []pushOutcome{}
	for _, out := range s.pushOutcomes[t.Workspace.ID] {
		if out.Watch == id && out.Lifetime == record.Lifetime && time.Since(out.At) < 24*time.Hour {
			outcomes = append(outcomes, out)
		}
	}
	s.pushMu.Unlock()
	cloudWrite(w, 200, map[string]any{"lastAcceptance": last, "recent": outcomes, "note": "Recent request outcomes are bounded to 100 per workspace, expire after 24 hours, and clear on restart. Last acceptance is durable. Notification delivery is separate."})
}
