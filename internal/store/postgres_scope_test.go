package store

import (
	"context"
	"testing"
)

func TestPostgresScopeIsLocalOnReusedConnection(t *testing.T) {
	s := openPostgresTestStore(t) // One connection forces pool reuse.
	ctx := context.Background()
	var originalPath string
	if err := s.db.QueryRowContext(ctx, "SHOW search_path").Scan(&originalPath); err != nil {
		t.Fatal(err)
	}
	for _, readOnly := range []bool{false, true} {
		tx, err := s.beginPostgres(ctx, readOnly)
		if err != nil {
			t.Fatal(err)
		}
		var path, workspace string
		err = tx.QueryRowContext(ctx, "SELECT current_setting('search_path'),current_setting('ding.workspace')").Scan(&path, &workspace)
		if err != nil || path != "ding_runtime,pg_catalog" || workspace != s.workspace {
			tx.Rollback()
			t.Fatal("incorrect transaction scope", err)
		}
		if readOnly {
			err = tx.Rollback()
		} else {
			err = tx.Commit()
		}
		if err != nil {
			t.Fatal(err)
		}
		err = s.db.QueryRowContext(ctx, "SELECT current_setting('search_path'),coalesce(current_setting('ding.workspace',true),'')").Scan(&path, &workspace)
		if err != nil || path != originalPath || workspace != "" {
			t.Fatal("transaction scope leaked into pooled connection", err)
		}
		var visible int
		if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM ding_runtime.workspaces").Scan(&visible); err != nil || visible != 0 {
			t.Fatal("unscoped pooled connection can see workspace rows", err)
		}
	}
}
