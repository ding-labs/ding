package cloud

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/cloud/state"
)

// DeleteOffline reapplies an independently recorded deletion to a stopped host
// or quarantined restore. It never constructs or starts an execution engine.
func DeleteOffline(ctx context.Context, dir, id string) error {
	if !filepath.IsAbs(dir) || !accountID.MatchString(id) {
		return fmt.Errorf("absolute data directory and exact workspace ID required")
	}
	if _, err := os.Stat(filepath.Join(dir, "control.db")); err != nil {
		return fmt.Errorf("existing cloud control database required: %w", err)
	}
	db, err := state.Open(ctx, dir)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.BeginAccountDelete(ctx, id); err != nil {
		return err
	}
	p := &Pool{root: dir, db: db, tenants: map[string]*Tenant{}, deleting: map[string]bool{}}
	if err := p.Delete(ctx, id); err != nil {
		return err
	}
	return db.FinishAccountDelete(ctx, id)
}
