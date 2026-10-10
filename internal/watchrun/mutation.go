package watchrun

import (
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
