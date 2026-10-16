package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func TestPreparedStatementsKeepWorkspaceScopeOnPoolReuse(t *testing.T) {
	ctx := context.Background()
	if err := MigratePostgres(ctx, postgresTestPool(t, "migration"), "ding_app"); err != nil {
		t.Fatal(err)
	}
	pool := postgresTestPool(t, "app", "cache_statement")
	var stores []*Store
	for i := range 2 {
		workspace := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("cached-%d-%d", i, time.Now().UnixNano()))))
		s, err := OpenPostgres(ctx, pool, workspace)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if err := s.Update(ctx, func(tx *Tx) error {
			_, err := tx.exec(statement{postgres: "INSERT INTO metadata(key,value) VALUES('cached-owner',$1)"}, workspace)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		stores = append(stores, s)
	}
	for range 10 {
		for _, s := range stores {
			if err := s.View(ctx, func(tx *Tx) error {
				var owner string
				if err := tx.sql.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key=$1", "cached-owner").Scan(&owner); err != nil {
					return err
				}
				if owner != s.workspace {
					t.Fatal("prepared statement crossed workspace scope")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	var prepared int
	if err := pool.QueryRowContext(ctx, "SELECT count(*) FROM pg_prepared_statements").Scan(&prepared); err != nil || prepared == 0 {
		t.Fatal("test did not exercise prepared statements", err)
	}
}
