package state

import (
	"context"
	"fmt"

	"github.com/ding-labs/ding/internal/identity"
)

// InitializeDisplayName uses a verified sign-in profile only while no name has
// been set. Later sign-ins, including linked identities, preserve that choice.
func (d *DB) InitializeDisplayName(ctx context.Context, user, raw string) error {
	name, err := identity.DisplayName(raw)
	if err != nil {
		return err
	}
	_, err = d.exec(ctx, qInitializeDisplayName, name, user)
	return err
}

func (d *DB) UpdateDisplayName(ctx context.Context, user, raw string) (User, error) {
	name, err := identity.DisplayName(raw)
	if err != nil {
		return User{}, err
	}
	tx, err := d.begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.query(qUpdateDisplayName), name, user)
	if err != nil {
		return User{}, err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return User{}, fmt.Errorf("account unavailable")
	}
	if err := audit(ctx, tx, user, "", "profile:update", user); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return d.User(ctx, user)
}
