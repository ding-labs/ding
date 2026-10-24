package state

import (
	"context"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/identity"
)

func (d *DB) RenameWorkspace(ctx context.Context, actor access.Principal, raw string) (Workspace, error) {
	name, err := identity.DisplayName(raw)
	if err != nil {
		return Workspace{}, err
	}
	if actor.Kind != "browser" {
		return Workspace{}, access.ErrDenied
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return Workspace{}, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, tx.query(qPrincipal1), actor.User, actor.Workspace).Scan(&actor.Role); err != nil || !actor.Allows(access.Owners) {
		return Workspace{}, access.ErrDenied
	}
	if _, err := tx.ExecContext(ctx, tx.query(qRenameWorkspace), name, actor.Workspace); err != nil {
		return Workspace{}, err
	}
	if err := audit(ctx, tx, actor.User, actor.Workspace, "workspace:rename", actor.Workspace); err != nil {
		return Workspace{}, err
	}
	if err := tx.Commit(); err != nil {
		return Workspace{}, err
	}
	return d.Workspace(ctx, actor.Workspace)
}
