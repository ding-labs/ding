package state

import (
	"context"

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
