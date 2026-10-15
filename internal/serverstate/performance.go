package state

import (
	"database/sql"
	"github.com/ding-labs/ding/internal/runtimeperf"
)

func (d *DB) PerfBackend() runtimeperf.Backend {
	if d.postgres {
		return runtimeperf.Postgres
	}
	return runtimeperf.SQLite
}
func (d *DB) ConnectionStats() (write, read sql.DBStats) {
	write = d.sql.Stats()
	if d.readers != nil {
		read = d.readers.Stats()
	}
	return
}
