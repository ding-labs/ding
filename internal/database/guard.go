package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// TransactionGuard locks authoritative authorization rows using the acceptance
// transaction and returns its pre-commit check. It cannot reenter the same pool.
type TransactionGuard struct {
	Pool *sql.DB
	Lock func(context.Context, *sql.Tx) (func() error, error)
}

var ErrAuthorization = errors.New("producer credential expired or unavailable")

type guardKey struct{}
type commitCheckKey struct{}

// WithCommitCheck validates process-fenced facts such as expiry after a writer
// wait and immediately before commit. It must not acquire the workspace pool.
func WithCommitCheck(ctx context.Context, check func() error) context.Context {
	return context.WithValue(ctx, commitCheckKey{}, check)
}

func WithTransactionGuard(ctx context.Context, g TransactionGuard) context.Context {
	if g.Lock == nil {
		return ctx
	}
	return context.WithValue(ctx, guardKey{}, g)
}

func LockTransactionGuard(ctx context.Context, tx *sql.Tx, pool *sql.DB) (func() error, error) {
	validate, _ := ctx.Value(commitCheckKey{}).(func() error)
	checkOnly := func() error {
		if validate != nil {
			return validate()
		}
		return nil
	}
	g, ok := ctx.Value(guardKey{}).(TransactionGuard)
	if !ok || g.Lock == nil {
		if err := checkOnly(); err != nil {
			return nil, err
		}
		return checkOnly, nil
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
	if err := checkOnly(); err != nil {
		return nil, err
	}
	return func() error {
		if err := check(); err != nil {
			return err
		}
		return checkOnly()
	}, nil
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
