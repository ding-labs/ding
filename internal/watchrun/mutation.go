package watchrun

import (
	"fmt"
	"github.com/ding-labs/ding/internal/store"
	"time"
)

// Runtime effects and their receipts share one SQLite transaction. Different
// callers supply their own authorization and durable idempotency boundaries.
type mutation interface {
	begin(*store.Tx, any, time.Time) (bool, error)
	finish(*store.Tx, any, time.Time) error
}
type noMutation struct{}

func (noMutation) begin(*store.Tx, any, time.Time) (bool, error) { return false, nil }
func (noMutation) finish(*store.Tx, any, time.Time) error        { return nil }

func checkHandoffHold(tx *store.Tx, id string, m mutation) error {
	held, err := tx.HandoffHold(id)
	if err != nil {
		return err
	}
	if held == "" {
		return nil
	}
	if owner, ok := m.(interface{ handoffID() string }); ok && owner.handoffID() == held {
		return nil
	}
	return fmt.Errorf("watch is held by transfer %s; reconcile the transfer before editing or resuming", held)
}
