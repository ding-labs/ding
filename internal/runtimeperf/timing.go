// Package runtimeperf records bounded process-local performance measurements.
// Backends and operations are enums; identifiers and payloads are never labels.
package runtimeperf

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

type Backend uint8

const (
	SQLite Backend = iota
	Postgres
	backendCount
)

type Operation uint8

const (
	WriterWait Operation = iota
	Begin
	Callback
	Commit
	Snapshot
	Evaluate
	Maintenance
	DeliveryClaim
	DeliveryIO
	Authorization
	ConditionCompile
	EnqueueFirst
	WorkspaceLock
	SQLSelect
	SQLInsert
	SQLUpdate
	SQLDelete
	SQLBegin
	SQLCommit
	SQLRollback
	SQLScope
	SQLOther
	SQLMaintenanceBegin
	WriterHold
	TransactionGuard
	PreCommit
	operationCount
)

var backends = [...]string{"sqlite", "postgres"}
var operations = [...]string{"writer_wait", "transaction_begin", "write_callback", "commit", "snapshot", "evaluation", "maintenance", "delivery_claim", "delivery_io", "authorization", "condition_compile", "enqueue_to_first_attempt", "workspace_row_lock", "sql_select", "sql_insert", "sql_update", "sql_delete", "sql_begin", "sql_commit", "sql_rollback", "sql_scope", "sql_other", "sql_maintenance_begin", "writer_hold", "transaction_guard", "pre_commit"}
var bounds = [...]time.Duration{time.Millisecond, 10 * time.Millisecond, 100 * time.Millisecond, time.Second, 10 * time.Second}
var labels = [...]string{"0.001", "0.01", "0.1", "1", "10"}

type histogram struct {
	count, sum atomic.Int64
	buckets    [5]atomic.Int64
}

var timing [backendCount][operationCount]histogram

func Observe(backend Backend, operation Operation, duration time.Duration) {
	if backend >= backendCount || operation >= operationCount || duration < 0 {
		return
	}
	h := &timing[backend][operation]
	h.sum.Add(int64(duration))
	h.count.Add(1)
	for i, bound := range bounds {
		if duration <= bound {
			h.buckets[i].Add(1)
		}
	}
}
func Since(backend Backend, operation Operation, start time.Time) {
	Observe(backend, operation, time.Since(start))
}
func Write(w io.Writer) {
	for backend, name := range backends {
		for operation, label := range operations {
			h := &timing[backend][operation]
			for i, bound := range labels {
				fmt.Fprintf(w, "ding_runtime_operation_seconds_bucket{backend=%q,operation=%q,le=%q} %d\n", name, label, bound, h.buckets[i].Load())
			}
			count := h.count.Load()
			fmt.Fprintf(w, "ding_runtime_operation_seconds_bucket{backend=%q,operation=%q,le=\"+Inf\"} %d\nding_runtime_operation_seconds_count{backend=%q,operation=%q} %d\nding_runtime_operation_seconds_sum{backend=%q,operation=%q} %g\n", name, label, count, name, label, count, name, label, float64(h.sum.Load())/1e9)
		}
	}
}
