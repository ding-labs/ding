package cloud

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

type scaleAccounting struct {
	Watermark int64
	Usage     store.PushUsage
	Rows      store.Usage
	Health    store.Health
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
			if err != nil {
				return err
			}
			a.Watermark, err = tx.ObservationWatermark()
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
	seen := map[[32]byte]bool{}
	ids := map[[32]byte]bool{}
	expected := make([]store.PushUsage, len(lab.tenants))
	spans := make([][]scaleSpan, len(lab.tenants))
	records := make([]map[string]store.WatchRecord, len(lab.tenants))
	grants := make([]state.Service, len(lab.tenants))
	for i, tenant := range lab.tenants {
		records[i] = map[string]store.WatchRecord{}
		for _, watchID := range c.watchIDs() {
			record, e := tenant.App.Record(context.Background(), watchID)
			if e != nil {
				t.Fatal(e)
			}
			records[i][watchID] = record
		}
		grants[i], err = lab.server.DB.ServiceToken(context.Background(), os.Getenv(fmt.Sprintf("DING_LOCAL_SCALE_%d", i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	problems := []string{}
	committed, acknowledged := 0, 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		var e pushLedgerEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if e.Stage != "offered" && e.Receipt == nil {
			continue
		}
		if e.Stage == "offered" {
			hash := sha256.Sum256([]byte(e.Key))
			if seen[hash] {
				problems = append(problems, "duplicate offered key")
			}
			seen[hash] = true
		}
		p := lab.producers[e.Producer]
		i, err := strconv.Atoi(strings.TrimPrefix(p.TokenEnv, "DING_LOCAL_SCALE_"))
		if err != nil {
			t.Fatal(err)
		}
		input := watchrun.NewPushInput(grants[i].ID, e.Key, p.Body, 1000000, 24*60*60*1000000000)
		r, err := lab.tenants[i].App.ReplayPush(context.Background(), records[i][scaleProducerWatch(p)], input)
		if errors.Is(err, store.ErrNotFound) {
			if e.Receipt != nil {
				problems = append(problems, "acknowledged receipt absent")
			}
			continue
		}
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if e.Stage == "offered" {
			id := sha256.Sum256([]byte(r.AcceptanceID))
			if ids[id] || r.First < 1 || r.Last < r.First || r.Last-r.First+1 > int64(c.Outputs+1) {
				problems = append(problems, "duplicate key or receipt range/identity mismatch")
			}
			ids[id] = true
			committed++
			expected[i].Requests++
			expected[i].Bytes += int64(len(p.Body))
			expected[i].Observations += r.Last - r.First + 1
			spans[i] = append(spans[i], scaleSpan{r.First, r.Last})
			if err := scaleVerifyReceipt(lab.tenants[i], r, c.Outputs); err != nil {
				problems = append(problems, err.Error())
			}
		} else {
			ack := e.Receipt
			acknowledged++
			if ack.AcceptanceID != r.AcceptanceID || ack.First != r.First || ack.Last != r.Last || ack.Revision != r.Revision || ack.Generation != r.Generation || ack.WatchID != r.WatchID {
				problems = append(problems, "response and durable receipt differ")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	resolved := committed - acknowledged
	if resolved < 0 {
		problems = append(problems, "more acknowledgments than durable offers")
	}
	after := scaleAccounts(t, lab)
	lab.internalObservations = 0
	for i, a := range after {
		b := before[i]
		delta := store.PushUsage{Requests: a.Usage.Requests - b.Usage.Requests, Bytes: a.Usage.Bytes - b.Usage.Bytes, Observations: a.Usage.Observations - b.Usage.Observations}
		internal, err := scaleVerifyCoverage(lab.tenants[i], before[i].Watermark, a.Watermark, spans[i])
		if err != nil {
			problems = append(problems, err.Error())
		}
		lab.internalObservations += internal
		if delta != expected[i] || int64(a.Rows.Observations-b.Rows.Observations) != expected[i].Observations+internal {
			problems = append(problems, fmt.Sprintf("workspace %d durable usage/rows differ", i))
		}
		if a.Health.Integrity != "ok" {
			problems = append(problems, "database integrity failure")
		}
	}
	return resolved, problems
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
	if errs := scaleChecks(t, lab, c, path); len(errs) > 0 {
		t.Fatal(errs)
	}
}
