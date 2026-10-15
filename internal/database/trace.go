package database

import (
	"context"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/runtimeperf"
	"github.com/jackc/pgx/v5"
)

// queryTrace counts attempted driver calls, including failed statements and
// ordinary read rollbacks. It never retains SQL or arguments. These are driver
// calls, not wire packets; protocol preparation may add network round trips.
type queryTrace struct{}
type queryTraceKey struct{}
type queryTiming struct {
	start     time.Time
	operation runtimeperf.Operation
}

func (queryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryTraceKey{}, queryTiming{time.Now(), queryOperation(data.SQL)})
}

func (queryTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if timing, ok := ctx.Value(queryTraceKey{}).(queryTiming); ok {
		runtimeperf.Since(runtimeperf.Postgres, timing.operation, timing.start)
	}
}

func queryOperation(query string) runtimeperf.Operation {
	query = strings.TrimSpace(query)
	end := strings.IndexAny(query, " \t\r\n;")
	if end >= 0 {
		query = query[:end]
	}
	switch strings.ToLower(query) {
	case "select":
		return runtimeperf.SQLSelect
	case "insert":
		return runtimeperf.SQLInsert
	case "update":
		return runtimeperf.SQLUpdate
	case "delete":
		return runtimeperf.SQLDelete
	case "begin":
		return runtimeperf.SQLBegin
	case "commit":
		return runtimeperf.SQLCommit
	case "rollback":
		return runtimeperf.SQLRollback
	case "set", "reset":
		return runtimeperf.SQLScope
	default:
		return runtimeperf.SQLOther
	}
}
