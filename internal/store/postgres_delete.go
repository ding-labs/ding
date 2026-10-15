package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ding-labs/ding/internal/database"
)

// DeletePostgresWorkspace runs only after admissions and that workspace's
// workers have stopped. Fixed table order respects every scoped foreign key.
func DeletePostgresWorkspace(ctx context.Context, pool *sql.DB, workspace string, owner *database.Ownership) error {
	if !postgresWorkspaceID.MatchString(workspace) {
		return fmt.Errorf("invalid PostgreSQL workspace")
	}
	s := &Store{db: pool, postgres: true, workspace: workspace, ownership: owner}
	tx, err := s.beginPostgres(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var found string
	err = tx.QueryRowContext(ctx, "SELECT workspace_id FROM workspaces WHERE workspace_id=$1 FOR UPDATE", workspace).Scan(&found)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	for _, table := range []string{"delivery_streams", "delivery_attempts", "event_replays", "event_evidence", "outbox", "push_last", "handoffs", "timers", "input_receipts", "entities", "events", "observations", "watches", "watch_revisions", "destinations", "destination_revisions", "integration_previews", "integration_operations", "integration_grants", "push_receipt_stats", "push_receipts", "push_usage", "audit", "metadata", "workspaces"} {
		if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE workspace_id=$1", workspace); err != nil {
			return err
		}
	}
	if err = owner.Fence(ctx, tx, true); err != nil {
		return err
	}
	return tx.Commit()
}
