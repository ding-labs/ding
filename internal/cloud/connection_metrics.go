package cloud

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/ding-labs/ding/internal/runtimeperf"
)

func addConnectionStats(total *sql.DBStats, next sql.DBStats) {
	total.MaxOpenConnections += next.MaxOpenConnections
	total.OpenConnections += next.OpenConnections
	total.InUse += next.InUse
	total.Idle += next.Idle
	total.WaitCount += next.WaitCount
	total.WaitDuration += next.WaitDuration
	total.MaxIdleClosed += next.MaxIdleClosed
	total.MaxIdleTimeClosed += next.MaxIdleTimeClosed
	total.MaxLifetimeClosed += next.MaxLifetimeClosed
}
func writeConnectionStats(w io.Writer, backend, role string, s sql.DBStats) {
	fmt.Fprintf(w, "ding_runtime_sql_connections_limit{backend=%q,role=%q} %d\nding_runtime_sql_connections_open{backend=%q,role=%q} %d\nding_runtime_sql_connections_in_use{backend=%q,role=%q} %d\nding_runtime_sql_connections_idle{backend=%q,role=%q} %d\nding_runtime_sql_waits_total{backend=%q,role=%q} %d\nding_runtime_sql_wait_seconds_total{backend=%q,role=%q} %g\n", backend, role, s.MaxOpenConnections, backend, role, s.OpenConnections, backend, role, s.InUse, backend, role, s.Idle, backend, role, s.WaitCount, backend, role, s.WaitDuration.Seconds())
	for _, closed := range []struct {
		reason string
		count  int64
	}{{"idle_limit", s.MaxIdleClosed}, {"idle_time", s.MaxIdleTimeClosed}, {"lifetime", s.MaxLifetimeClosed}} {
		fmt.Fprintf(w, "ding_runtime_sql_connections_closed_total{backend=%q,role=%q,reason=%q} %d\n", backend, role, closed.reason, closed.count)
	}
}
func (p *Pool) connectionMetrics(w io.Writer, list []*Tenant) {
	runtimeperf.Write(w)
	if pg, ok := p.storage.(postgresWorkspaces); ok {
		writeConnectionStats(w, "postgres", "shared", pg.pool.Stats())
		writeConnectionStats(w, "postgres", "ownership", pg.owner.ConnectionStats())
	} else {
		write, read := p.db.ConnectionStats()
		writeConnectionStats(w, "sqlite", "control_writer", write)
		writeConnectionStats(w, "sqlite", "credential_readers", read)
		var writers, readers sql.DBStats
		for _, t := range list {
			write, read := t.App.Store.ConnectionStats()
			addConnectionStats(&writers, write)
			addConnectionStats(&readers, read)
		}
		writeConnectionStats(w, "sqlite", "workspace_writers", writers)
		writeConnectionStats(w, "sqlite", "snapshot_readers", readers)
	}
	acquisition, delivery := 0, 0
	for _, t := range list {
		acquisition += t.App.AcquisitionWorkers
		delivery += t.App.DeliveryWorkers
	}
	fmt.Fprintf(w, "ding_runtime_worker_limit{kind=\"acquisition\"} %d\nding_runtime_worker_limit{kind=\"delivery\"} %d\n", acquisition, delivery)
}
