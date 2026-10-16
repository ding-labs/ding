package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

// OpenPostgres opens one bounded pool for the instance. Tenant facades borrow
// transactions from it; they must never close it or create another tenant pool.
func OpenPostgres(ctx context.Context, c Config, lookup func(string) (string, bool)) (*sql.DB, error) {
	if c.Effective().Backend != "postgres" {
		return nil, fmt.Errorf("PostgreSQL backend must be selected explicitly")
	}
	dsn, err := c.ResolveDSN(lookup)
	if err != nil {
		return nil, err
	}
	cfg, err := postgresConfig(dsn)
	if err != nil {
		return nil, err
	}
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(c.Effective().MaxOpenConns)
	db.SetMaxIdleConns(min(2, c.Effective().MaxOpenConns))
	db.SetConnMaxIdleTime(time.Minute)
	db.SetConnMaxLifetime(5 * time.Minute)
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(probe); err != nil {
		db.Close()
		return nil, SafeError("connect", err)
	}
	return db, nil
}

func postgresConfig(dsn string) (*pgx.ConnConfig, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid PostgreSQL DSN")
	}
	local := strings.HasPrefix(cfg.Host, "/") || net.ParseIP(cfg.Host).IsLoopback() || cfg.Host == "localhost"
	if !local {
		if cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify {
			return nil, fmt.Errorf("remote PostgreSQL requires verified TLS (sslmode=verify-full)")
		}
		for _, fallback := range cfg.Fallbacks {
			if fallback.TLSConfig == nil || fallback.TLSConfig.InsecureSkipVerify {
				return nil, fmt.Errorf("PostgreSQL TLS downgrade fallback is forbidden")
			}
		}
	}
	cfg.ConnectTimeout = 5 * time.Second
	// Extended protocol without a session statement cache also works through
	// transaction poolers. Workspace scope is always transaction-local.
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.RuntimeParams["application_name"] = "ding"
	cfg.RuntimeParams["statement_timeout"] = "30000"
	cfg.RuntimeParams["lock_timeout"] = "5000"
	cfg.RuntimeParams["idle_in_transaction_session_timeout"] = "30000"
	cfg.RuntimeParams["synchronous_commit"] = "on"
	cfg.RuntimeParams["bytea_output"] = "hex"
	return cfg, nil
}

// SafeError retains a finite diagnostic category without exposing connection
// strings, server details, SQL values, or credentials in a libpq error message.
func SafeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return fmt.Errorf("PostgreSQL %s failed (SQLSTATE %s)", operation, pg.Code)
	}
	return fmt.Errorf("PostgreSQL %s failed", operation)
}
