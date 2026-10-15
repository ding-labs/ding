package cloud

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

type scaleSpan struct{ first, last int64 }

func scaleVerifyReceipt(tenant *Tenant, r store.PushReceipt, outputs int) error {
	return tenant.App.Store.View(context.Background(), func(tx *store.Tx) error {
		rows, err := tx.ObservationRange(r.WatchID, r.First, r.Last)
		if err != nil {
			return err
		}
		if int64(len(rows)) != r.Last-r.First+1 {
			return fmt.Errorf("receipt observation range is incomplete")
		}
		healthy := 0
		for index, row := range rows {
			expected := r.AcceptanceID + ":" + strconv.Itoa(index) + ":" + strconv.FormatInt(r.AcceptedAt.UnixNano(), 10)
			if row.Sequence != r.First+int64(index) || row.InputID != expected || !row.AcceptedAt.Equal(r.AcceptedAt) {
				return fmt.Errorf("observation provenance differs from receipt")
			}
			if row.Health == "ok" {
				healthy++
			} else if index != 0 || row.Health != "gap" || row.Detail != "backpressure_gap" {
				return fmt.Errorf("unexpected observation inside push receipt")
			}
		}
		if healthy != outputs {
			return fmt.Errorf("projected output count differs from receipt")
		}
		return nil
	})
}

func scaleReadSpan(tenant *Tenant, first, last int64) ([]watch.Observation, error) {
	var rows []watch.Observation
	err := tenant.App.Store.View(context.Background(), func(tx *store.Tx) error { var err error; rows, err = tx.ObservationRange("", first, last); return err })
	return rows, err
}

// Every allocation after the baseline must be covered by an offered receipt or
// an explicitly recognized internal clock observation. Unexplained rows and
// missing allocations remain reconciliation failures.
func scaleVerifyCoverage(tenant *Tenant, before, after int64, spans []scaleSpan) (int64, error) {
	sort.Slice(spans, func(i, j int) bool { return spans[i].first < spans[j].first })
	next := before + 1
	var internal int64
	gap := func(first, last int64) error {
		for first <= last {
			end := min(last, first+999)
			rows, err := scaleReadSpan(tenant, first, end)
			if err != nil {
				return err
			}
			if int64(len(rows)) != end-first+1 {
				return fmt.Errorf("uncovered or missing observation allocations")
			}
			for index, row := range rows {
				if row.Sequence != first+int64(index) || row.Health != "clock" || row.Detail != "clock_discontinuity" || !strings.HasPrefix(row.InputID, "clock:") {
					return fmt.Errorf("unrecognized observation outside offered receipts")
				}
				internal++
			}
			first = end + 1
		}
		return nil
	}
	for _, span := range spans {
		if span.first < next || span.last < span.first || span.last > after {
			return internal, fmt.Errorf("receipt ranges overlap or cross the allocation watermark")
		}
		if err := gap(next, span.first-1); err != nil {
			return internal, err
		}
		next = span.last + 1
	}
	err := gap(next, after)
	return internal, err
}

func TestScaleReconcilesClockRowsAndAcceptedBackpressureGaps(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			c := scaleConfig{Name: "provenance", Backend: backend, PoolLimit: 1, Rate: 10, Seconds: 1}
			if err := c.validate(); err != nil {
				t.Fatal(err)
			}
			lab := newScaleLab(t, c)
			before := scaleAccounts(t, lab)
			tenant := lab.tenants[0]
			// Synthetic persisted rows exercise the evidence reader. These are not
			// production fault injection or measured capacity samples.
			appendClock := func(id string) {
				t.Helper()
				if err := tenant.App.Store.Update(context.Background(), func(tx *store.Tx) error {
					r, err := tx.Watch("ci")
					if err != nil {
						return err
					}
					_, err = tx.AppendObservation("ci", r.Plan.Revision, r.Generation, watch.Observation{InputID: id, Health: "clock", Detail: "clock_discontinuity", AcceptedAt: time.Now()})
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			appendClock("clock:before")
			if err := tenant.App.Store.Update(context.Background(), func(tx *store.Tx) error {
				r, err := tx.Watch("ci")
				if err != nil {
					return err
				}
				r.LastError = "local backpressure recovery fixture"
				return tx.SaveWatch(r, time.Now())
			}); err != nil {
				t.Fatal(err)
			}
			ledger := t.TempDir() + "/ledger"
			result, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Duration: time.Second, Rate: 10, Ledger: ledger, LedgerBatch: 8, CompactLedger: true, NoRetry: true})
			if err != nil || result.Accepted != 10 {
				t.Fatal(result, err)
			}
			appendClock("clock:after")
			if resolved, problems := scaleReconcile(t, lab, c, ledger, before); resolved != 0 || len(problems) > 0 || lab.internalObservations != 2 {
				t.Fatal(resolved, problems, lab.internalObservations)
			}
			if err = tenant.App.Store.Update(context.Background(), func(tx *store.Tx) error {
				r, e := tx.Watch("ci")
				if e != nil {
					return e
				}
				_, e = tx.AppendObservation("ci", r.Plan.Revision, r.Generation, watch.Observation{InputID: "unrecognized", Health: "unknown", AcceptedAt: time.Now()})
				return e
			}); err != nil {
				t.Fatal(err)
			}
			if _, problems := scaleReconcile(t, lab, c, ledger, before); len(problems) == 0 {
				t.Fatal("unrecognized rows were silently classified as internal")
			}
		})
	}
}
