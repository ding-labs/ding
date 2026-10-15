package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueueIndexesAndSchemaTwoUpgrade(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "ding.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(initialSchema + inspectionSchema + "INSERT INTO metadata(key,value) VALUES('sentinel','present'); PRAGMA user_version=2;"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s := openTest(t, dir)
	var value string
	if err := s.db.QueryRow("SELECT value FROM metadata WHERE key='sentinel'").Scan(&value); err != nil || value != "present" {
		t.Fatal(value, err)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, fmt.Sprintf("before-schema-%d-*.db", SchemaVersion)))
	if len(backups) != 1 {
		t.Fatal("schema 2 must receive a backup", backups)
	}
	rows, err := s.db.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+claimQuery, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"delivery_stream_active", "outbox_live_order", "outbox_stream_lease"} {
		if !strings.Contains(plan.String(), index) {
			t.Fatalf("queue would scan historical deliveries; missing %s:\n%s", index, plan.String())
		}
	}
	if strings.Contains(plan.String(), "TEMP B-TREE") {
		t.Fatal("queue scans and sorts all candidates", plan.String())
	}
}

func TestRetentionBatchesUseTimeIndexes(t *testing.T) {
	s := openTest(t, t.TempDir())
	indexes := map[string]string{"events": "events_retention", "observations": "observations_retention", "watches": "watches_deleted_retention", "watch_revisions": "watch_revision_retention", "destination_revisions": "destination_revision_retention"}
	for _, family := range retentionFamilies {
		args := []any{int64(100), int64(-1), any("")}
		if family.numeric {
			args[2] = int64(0)
		}
		if family.secondKey {
			args = append(args, "")
		}
		args = append(args, 64)
		rows, err := s.db.Query("EXPLAIN QUERY PLAN "+family.selectQuery.sqlite, args...)
		if err != nil {
			t.Fatal(err)
		}
		var details strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			details.WriteString(detail)
			details.WriteByte('\n')
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(details.String(), indexes[family.name]) || strings.Contains(details.String(), "TEMP B-TREE") {
			t.Fatal("retention scans or sorts history", details.String())
		}
	}
}
