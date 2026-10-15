package database

import (
	"context"
	"database/sql"
	"fmt"
)

// TransactionGuard locks authoritative authorization rows using the acceptance
// transaction and returns its pre-commit check. It cannot reenter the same pool.
type TransactionGuard struct {
	Pool *sql.DB
	Lock func(context.Context, *sql.Tx) (func() error, error)
}
type guardKey struct{}

func WithTransactionGuard(ctx context.Context, g TransactionGuard) context.Context {
	if g.Lock == nil {
		return ctx
	}
	return context.WithValue(ctx, guardKey{}, g)
}

func LockTransactionGuard(ctx context.Context, tx *sql.Tx, pool *sql.DB) (func() error, error) {
	g, ok := ctx.Value(guardKey{}).(TransactionGuard)
	if !ok || g.Lock == nil {
		return func() error { return nil }, nil
	}
	if g.Pool != pool {
		return nil, fmt.Errorf("authorization guard belongs to another database instance")
	}
	check, err := g.Lock(ctx, tx)
	if err != nil {
		return nil, err
	}
	if check == nil {
		check = func() error { return nil }
	}
	return check, nil
}
