package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestBoundedPostgresPoolSurvivesIdleBurst(t *testing.T) {
	f := pgtest.New(t)
	f.SetAppEnvironment(t, "DING_POOL_TEST")
	pool, err := database.OpenPostgres(context.Background(), database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "DING_POOL_TEST"}, MaxOpenConns: 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for range 2 {
		var held []*sql.Conn
		for range 4 {
			conn, err := pool.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			held = append(held, conn)
		}
		for _, conn := range held {
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
		}
		stats := pool.Stats()
		if stats.OpenConnections != 4 || stats.Idle != 4 || stats.MaxIdleClosed != 0 {
			t.Fatal("bounded pool churned between bursts", stats)
		}
	}
}
