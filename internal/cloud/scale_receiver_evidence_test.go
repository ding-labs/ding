package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/store"
)

// Read every delivered record before taking the receiver snapshot. A delivery
// committed by this point must already have a durable receiver record, even if
// later attempts are still arriving in an intentionally overloaded case.
func scaleReceiverEvidence(t *testing.T, lab *scaleLab, out string) (scaleReceiverState, []string) {
	t.Helper()
	delivered := map[string]bool{}
	for _, tenant := range lab.tenants {
		if err := tenant.App.Store.View(context.Background(), func(tx *store.Tx) error {
			q := store.ConsoleQuery{Limit: 100, Status: "delivered"}
			for {
				page, err := tx.ConsoleDeliveries(q)
				if err != nil {
					return err
				}
				for _, delivery := range page.Deliveries {
					delivered[delivery.EventID] = true
				}
				if !page.More {
					break
				}
				q.Cursor = page.Cursor
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	state, raw := lab.receiver.snapshot(t)
	ledgerIDs := map[string]bool{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for {
		var record struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		err := decoder.Decode(&record)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal("invalid receiver ledger", err)
		}
		ledgerIDs[record.Data.ID] = true
	}
	problems := []string{}
	for id := range delivered {
		if !state.IDs[id] || !ledgerIDs[id] {
			problems = append(problems, "delivered event absent from durable receiver evidence")
			break
		}
	}
	for id := range state.IDs {
		if !ledgerIDs[id] {
			problems = append(problems, "receiver acknowledgement absent from durable ledger")
			break
		}
	}
	file, err := mcpconfig.CreatePrivate(filepath.Join(out, "receiver.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Write(raw)
	if err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closed != nil {
		t.Fatal(closed)
	}
	return state, problems
}

func TestScaleReceiverEvidenceIncludesAllDeliveryPages(t *testing.T) {
	c := scaleConfig{Name: "receiver-evidence", Rate: 100, Seconds: 2, Watches: 4, DeliveryWorkers: 4, Condition: "alerts", Receiver: "fast"}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	// This is pagination correctness, so send a fixed number without a
	// timed arrival gate that depends on race-detector/CI machine speed.
	for i := range 200 {
		p := lab.producers[i%len(lab.producers)]
		if status, _ := scaleSend(t, lab, p, fmt.Sprintf("page-%d", i), p.Body); status != 202 {
			t.Fatal("fixture acceptance", status)
		}
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		pending := 0
		for _, a := range scaleAccounts(t, lab) {
			pending += a.Rows.Pending
		}
		if pending == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	state, problems := scaleReceiverEvidence(t, lab, t.TempDir())
	if len(problems) > 0 || len(state.IDs) != 200 {
		t.Fatal(problems, len(state.IDs))
	}
	lab.receiver.mu.Lock()
	for id := range lab.receiver.ids {
		delete(lab.receiver.ids, id)
		break
	}
	lab.receiver.mu.Unlock()
	if _, problems = scaleReceiverEvidence(t, lab, t.TempDir()); len(problems) == 0 {
		t.Fatal("missing receiver acknowledgement escaped reconciliation")
	}
}
