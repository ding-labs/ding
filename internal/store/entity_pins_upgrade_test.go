package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPopulatedEntityReferenceUpgrade(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			var raw *sql.DB
			var tx *Tx
			var upgrade func() error
			var read func() *Tx
			if backend == "sqlite" {
				dir := t.TempDir()
				var err error
				raw, err = sql.Open("sqlite", filepath.Join(dir, "ding.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer raw.Close()
				if _, err = raw.Exec(initialSchema + inspectionSchema + queueIndexSchema + integrationSchema + handoffSchema + auditSchema + pushSchema + pushStatsSchema + deliveryStreamsSchema + "PRAGMA user_version=9;"); err != nil {
					t.Fatal(err)
				}
				r, err := raw.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				tx = &Tx{ctx: ctx, sql: r, writable: true}
				upgrade = func() error {
					s, err := Open(ctx, dir)
					if err != nil {
						return err
					}
					return s.Close()
				}
				read = func() *Tx {
					r, err := raw.BeginTx(ctx, nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { r.Rollback() })
					return &Tx{ctx: ctx, sql: r}
				}
			} else {
				f := pgtest.New(t)
				old := []string{postgresRecordsSchema, postgresStateSchema, postgresEventsSchema, postgresPushSchema, postgresIntegrationsSchema, postgresStorageSchema, postgresCanonicalAccountingSchema, postgresDeliveryStreamsSchema}
				if err := database.Migrate(ctx, f.Migration, "runtime", old, "ding_app"); err != nil {
					t.Fatal(err)
				}
				tx = postgresScopedTx(t, f.App, "upgrade-pins")
				upgrade = func() error { return MigratePostgres(ctx, f.Migration, "ding_app") }
				read = func() *Tx { return postgresScopedTx(t, f.App, "upgrade-pins") }
			}
			c, _ := fixture(t)
			if err := tx.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running"}, testNow); err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(map[string]any{"lastSequence": 1, "freshSequence": 1, "baselineSequence": 2, "evidence": []int{2, 3}, "samples": []map[string]any{{"sequence": 4}}, "incidentEvent": "incident", "healthEvent": "health"})
			if _, err := tx.exec(qSaveEntity, "health", "[]", c.Revision, data, timestamp(testNow)); err != nil {
				t.Fatal(err)
			}
			if err := tx.sql.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := upgrade(); err != nil {
				t.Fatal(err)
			}
			r := read()
			rows, err := r.sql.QueryContext(ctx, r.query(qEntityPins), "health", "[]")
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			got := map[entityPin]bool{}
			for rows.Next() {
				var p entityPin
				if err = rows.Scan(&p.kind, &p.value); err != nil {
					t.Fatal(err)
				}
				got[p] = true
			}
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
			if len(got) != 6 || !got[entityPin{"input", "1"}] || !got[entityPin{"input", "4"}] || !got[entityPin{"event", "incident"}] || !got[entityPin{"event", "health"}] {
				t.Fatal("populated upgrade lost references", got)
			}
		})
	}
}
