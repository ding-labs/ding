package cloud

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestScalePostgresHTTPAccountingAndRestart(t *testing.T) {
	c := scaleConfig{Name: "postgres-smoke", Backend: "postgres", PoolLimit: 1, Rate: 10, Seconds: 1, Workspaces: 2, Outputs: 18}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	before := scaleAccounts(t, lab)
	ledger := filepath.Join(t.TempDir(), "requests.jsonl")
	result, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Duration: time.Second, Rate: 10, Ledger: ledger, LedgerBatch: 8, CompactLedger: true, NoRetry: true})
	if err != nil || result.Accepted != 10 {
		t.Fatal("real PostgreSQL HTTP path", err)
	}
	if resolved, problems := scaleReconcile(t, lab, c, ledger, before); resolved != 0 || len(problems) != 0 {
		t.Fatal(resolved, problems)
	}
	scaleRestart(t, lab, c)
	if problems := scaleChecks(t, lab, c, ledger); len(problems) != 0 {
		t.Fatal(problems)
	}
}
