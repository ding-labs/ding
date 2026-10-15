package database

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/runtimeperf"
	"github.com/jackc/pgx/v5"
)

func TestQueryTraceBoundsLabelsAndCountsFailures(t *testing.T) {
	queries := map[string]runtimeperf.Operation{
		" SELECT secret FROM private_tenant":        runtimeperf.SQLSelect,
		"insert\ninto private_tenant values ($1)":   runtimeperf.SQLInsert,
		"UPDATE private_tenant SET secret=$1":       runtimeperf.SQLUpdate,
		"delete from private_tenant":                runtimeperf.SQLDelete,
		"begin isolation level repeatable read":     runtimeperf.SQLBegin,
		"COMMIT;":                                   runtimeperf.SQLCommit,
		"rollback":                                  runtimeperf.SQLRollback,
		"SET LOCAL ding.workspace='private_tenant'": runtimeperf.SQLScope,
		"WITH private_tenant AS (SELECT 'secret') SELECT * FROM private_tenant": runtimeperf.SQLOther,
		"": runtimeperf.SQLOther,
	}
	rollbackCount := func() int64 {
		var out bytes.Buffer
		runtimeperf.Write(&out)
		prefix := `ding_runtime_operation_seconds_count{backend="postgres",operation="sql_rollback"} `
		for _, line := range strings.Split(out.String(), "\n") {
			if value, ok := strings.CutPrefix(line, prefix); ok {
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				return n
			}
		}
		t.Fatal("missing rollback metric")
		return 0
	}
	before := rollbackCount()
	trace := queryTrace{}
	for query, want := range queries {
		if got := queryOperation(query); got != want {
			t.Fatalf("classification = %d; want %d", got, want)
		}
		ctx := trace.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: query, Args: []any{"secret"}})
		trace.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: errors.New("secret")})
	}
	trace.TraceQueryEnd(context.Background(), nil, pgx.TraceQueryEndData{})
	var out bytes.Buffer
	runtimeperf.Write(&out)
	if strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "private_tenant") {
		t.Fatal("SQL or arguments leaked into metrics")
	}
	if rollbackCount()-before != 1 {
		t.Fatal("failed statement was not counted")
	}
}
