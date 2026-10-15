package store

import (
	"database/sql"
	"github.com/ding-labs/ding/internal/runtimeperf"
)

func (s *Store) PerfBackend() runtimeperf.Backend {
	if s.postgres {
		return runtimeperf.Postgres
	}
	return runtimeperf.SQLite
}

// ConnectionStats does not acquire a database connection or query tenant data.
// PostgreSQL facades report the shared pool; callers must count it only once.
func (s *Store) ConnectionStats() (write, read sql.DBStats) {
	write = s.db.Stats()
	if s.readers != nil {
		read = s.readers.Stats()
	}
	return
}
