package cloud

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

type scaleAccounting struct {
	Usage  store.PushUsage
	Rows   store.Usage
	Health store.Health
}

func scaleAccounts(t *testing.T, lab *scaleLab) []scaleAccounting {
	t.Helper()
	out := []scaleAccounting{}
	for _, tenant := range lab.tenants {
		a := scaleAccounting{}
		if err := tenant.App.Store.View(context.Background(), func(tx *store.Tx) error {
			var err error
			a.Usage, err = tx.PushUsage()
			if err != nil {
				return err
			}
			a.Rows, err = tx.Usage()
			return err
		}); err != nil {
			t.Fatal(err)
		}
		var err error
		a.Health, err = tenant.App.Store.Health(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

// Resolve every offered key against durable receipts, including requests whose
// network response was lost. Compare exact bytes/counts, not just HTTP totals.
func scaleReconcile(t *testing.T, lab *scaleLab, c scaleConfig, path string, before []scaleAccounting) (int, []string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	seen := map[string]bool{}
	ids := map[string]bool{}
	expected := make([]store.PushUsage, len(lab.tenants))
	responses := map[string]store.PushReceipt{}
	offers := []pushLedgerEntry{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		var e pushLedgerEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if e.Stage == "offered" {
			if seen[e.Key] {
				t.Fatal("duplicate offered key")
			}
			seen[e.Key] = true
			offers = append(offers, e)
		}
		if e.Receipt != nil {
			responses[e.Key] = *e.Receipt
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	errors := []string{}
	resolved := 0
	for _, e := range offers {
		p := lab.producers[e.Producer]
		i, err := strconv.Atoi(strings.TrimPrefix(p.TokenEnv, "DING_LOCAL_SCALE_"))
		if err != nil {
			t.Fatal(err)
		}
		tenant := lab.tenants[i]
		record, err := tenant.App.Record(context.Background(), "ci")
		if err != nil {
			t.Fatal(err)
		}
		grant, err := lab.server.DB.ServiceToken(context.Background(), os.Getenv(p.TokenEnv))
		if err != nil {
			t.Fatal(err)
		}
		input := watchrun.NewPushInput(grant.ID, e.Key, p.Body, 1000000, 24*60*60*1000000000)
		r, err := tenant.App.ReplayPush(context.Background(), record, input)
		if err == store.ErrNotFound {
			if _, ok := responses[e.Key]; ok {
				errors = append(errors, "acknowledged receipt absent")
			}
			continue
		}
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		if r.Last-r.First+1 != int64(c.Outputs) || ids[r.AcceptanceID] {
			errors = append(errors, "receipt range/identity mismatch")
		}
		ids[r.AcceptanceID] = true
		if ack, ok := responses[e.Key]; ok {
			if ack.AcceptanceID != r.AcceptanceID || ack.First != r.First || ack.Last != r.Last {
				errors = append(errors, "response and durable receipt differ")
			}
		} else {
			resolved++
		}
		expected[i].Requests++
		expected[i].Bytes += int64(len(p.Body))
		expected[i].Observations += int64(c.Outputs)
	}
	after := scaleAccounts(t, lab)
	for i, a := range after {
		b := before[i]
		delta := store.PushUsage{Requests: a.Usage.Requests - b.Usage.Requests, Bytes: a.Usage.Bytes - b.Usage.Bytes, Observations: a.Usage.Observations - b.Usage.Observations}
		if delta != expected[i] || int64(a.Rows.Observations-b.Rows.Observations) != expected[i].Observations {
			errors = append(errors, fmt.Sprintf("workspace %d durable usage/rows differ", i))
		}
		if a.Health.Integrity != "ok" {
			errors = append(errors, "database integrity failure")
		}
	}
	return resolved, errors
}

func TestScaleReconcilesFullPushLedger(t *testing.T) {
	c := scaleConfig{Name: "reconcile", Rate: 10, Seconds: 1, Workspaces: 2, Outputs: 18}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	before := scaleAccounts(t, lab)
	path := t.TempDir() + "/ledger"
	r, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Duration: 1000000000, Rate: 10, Ledger: path, LedgerBatch: 8, CompactLedger: true, NoRetry: true})
	if err != nil || r.Accepted != 10 {
		t.Fatal(r, err)
	}
	resolved, errs := scaleReconcile(t, lab, c, path, before)
	if resolved != 0 || len(errs) > 0 {
		t.Fatal(resolved, errs)
	}
}
