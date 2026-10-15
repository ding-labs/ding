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

type queryScopeKey struct{}
type queryScope struct {
	pool *sql.DB
	tx   *sql.Tx
}
type QueryExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func WithTransaction(ctx context.Context, pool *sql.DB, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, queryScopeKey{}, queryScope{pool, tx})
}
func Executor(ctx context.Context, pool *sql.DB) QueryExecutor {
	if s, ok := ctx.Value(queryScopeKey{}).(queryScope); ok && s.pool == pool {
		return s.tx
	}
	return pool
}
