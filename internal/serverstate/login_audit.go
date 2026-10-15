package state

import "context"

// LoginFailure records only a digest of the claimed username or provider name.
// Passwords, authorization codes, tokens and unverified claims never enter audit.
func (d *DB) LoginFailure(ctx context.Context, claimed string) error {
	tx, err := d.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := audit(ctx, tx, "unauthenticated", "", "login:failed", TokenHash(claimed)); err != nil {
		return err
	}
	return tx.Commit()
}
