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
)

const scaleLedgerChunkEntries = 4096

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
	producerWorkspaces := make([]int, len(lab.producers))
	digests := make([]string, len(lab.producers))
	outputsByProducer := make([]int, len(lab.producers))
	for index, p := range lab.producers {
		i, err := strconv.Atoi(strings.TrimPrefix(p.TokenEnv, "DING_LOCAL_SCALE_"))
		if err != nil {
			t.Fatal(err)
		}
		producerWorkspaces[index] = i
		outputsByProducer[index] = scaleProducerOutputs(p)
		digests[index] = fmt.Sprintf("%x", sha256.Sum256(p.Body))
	}
	consume := func(tx *store.Tx, i int, current map[string]store.WatchRecord, e pushLedgerEntry) {
		if e.Stage == "offered" || e.Stage == "planned" {
			hash := sha256.Sum256([]byte(e.Key))
			if seen[hash] {
				problems = append(problems, "duplicate offered key")
			}
			seen[hash] = true
		}
		p := lab.producers[e.Producer]
		outputs := outputsByProducer[e.Producer]
		watchID := scaleProducerWatch(p)
		rc, known := current[watchID]
		if !known {
			var err error
			rc, err = tx.Watch(watchID)
			if err != nil {
				problems = append(problems, err.Error())
				return
			}
			current[watchID] = rc
		}
		if rc.Status == "deleted" || rc.Lifetime != records[i][watchID].Lifetime {
			problems = append(problems, store.ErrStale.Error())
			return
		}
		r, err := tx.PushReceipt(rc.Lifetime, grants[i].ID, e.Key, digests[e.Producer], lab.tenants[i].App.Now())
		if errors.Is(err, store.ErrNotFound) {
			if e.Receipt != nil {
				problems = append(problems, "acknowledged receipt absent")
			}
			return
		}
		if err != nil {
			problems = append(problems, err.Error())
			return
		}
		if e.Stage == "offered" || e.Stage == "planned" {
			id := sha256.Sum256([]byte(r.AcceptanceID))
			if ids[id] || r.First < 1 || r.Last < r.First || r.Last-r.First+1 > int64(outputs+1) {
				problems = append(problems, "duplicate key or receipt range/identity mismatch")
			}
			ids[id] = true
			committed++
			expected[i].Requests++
			expected[i].Bytes += int64(len(p.Body))
			expected[i].Observations += r.Last - r.First + 1
			spans[i] = append(spans[i], scaleSpan{r.First, r.Last})
			if err := scaleVerifyReceiptTx(tx, r, outputs); err != nil {
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
	// Keep every key/provenance check, but amortize transaction setup within a
	// bounded tenant snapshot. Per-key transactions can outlast the load itself
	// when many idle tenants contend for a small PostgreSQL query pool.
	chunk := map[int][]pushLedgerEntry{}
	flush := func() {
		for i, tenant := range lab.tenants {
			entries := chunk[i]
			if len(entries) == 0 {
				continue
			}
			if err := tenant.App.Store.View(context.Background(), func(tx *store.Tx) error {
				current := map[string]store.WatchRecord{}
				for _, entry := range entries {
					consume(tx, i, current, entry)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		chunk = map[int][]pushLedgerEntry{}
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	lines := 0
	for scanner.Scan() {
		var e pushLedgerEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if (e.Stage == "offered" || e.Stage == "planned") || e.Receipt != nil {
			if e.Producer < 0 || e.Producer >= len(lab.producers) {
				t.Fatal("ledger producer is outside the fixture")
			}
			i := producerWorkspaces[e.Producer]
			chunk[i] = append(chunk[i], e)
		}
		lines++
		if lines == scaleLedgerChunkEntries {
			flush()
			lines = 0
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	flush()
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
	r, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Duration: 1000000000, Rate: 10, Ledger: path, LedgerBatch: 8, CompactLedger: true, PreplannedLedger: true, NoRetry: true})
	if err != nil || r.Accepted != 10 || r.AcceptedObservations != 180 || r.AcceptedBytes != int64(10*len(lab.producers[0].Body)) || r.Payloads[""].Accepted != 10 {
		t.Fatal(r, err)
	}
	resolved, errs := scaleReconcile(t, lab, c, path, before)
	if resolved != 0 || len(errs) > 0 {
		t.Fatal(resolved, errs)
	}
	if errs := scaleChecks(t, lab, c, path); len(errs) > 0 {
		t.Fatal(errs)
	}
	// Simulate losing every asynchronously journaled response after a driver
	// crash. The durable plan must still recover every accepted receipt.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var entry pushLedgerEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Stage != "response" {
			plan.WriteString(line + "\n")
		}
	}
	if err := os.WriteFile(path, []byte(plan.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if recovered, errs := scaleReconcile(t, lab, c, path, before); recovered != 10 || len(errs) != 0 {
		t.Fatal(recovered, errs)
	}
}

func TestScaleReconcileRetainsChecksAcrossChunks(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			c := scaleConfig{Name: "chunks", Backend: backend, PoolLimit: 1, Rate: 2, Seconds: 1, Workspaces: 2}
			if err := c.validate(); err != nil {
				t.Fatal(err)
			}
			lab := newScaleLab(t, c)
			before := scaleAccounts(t, lab)
			path := t.TempDir() + "/ledger"
			result, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Duration: 1000000000, Rate: 2, Ledger: path, LedgerBatch: 1, CompactLedger: true, NoRetry: true})
			if err != nil || result.Accepted != 2 {
				t.Fatal(result, err)
			}
			if resolved, problems := scaleReconcile(t, lab, c, path, before); resolved != 0 || len(problems) != 0 {
				t.Fatal(resolved, problems)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var offered, ack pushLedgerEntry
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				var entry pushLedgerEntry
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					t.Fatal(err)
				}
				if entry.Stage == "offered" {
					offered = entry
				}
				if entry.Receipt != nil {
					ack = entry
				}
			}
			// Corrupt only the private evidence, across a bounded snapshot edge.
			// No synthetic rows or acceptances are added to the service.
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			encoder := json.NewEncoder(file)
			for range scaleLedgerChunkEntries {
				if err = encoder.Encode(pushLedgerEntry{Stage: "diagnostic"}); err != nil {
					t.Fatal(err)
				}
			}
			if err = encoder.Encode(offered); err != nil {
				t.Fatal(err)
			}
			ack.Receipt.First++
			ack.Receipt.Last++
			if err = encoder.Encode(ack); err != nil {
				t.Fatal(err)
			}
			if err = file.Close(); err != nil {
				t.Fatal(err)
			}
			_, problems := scaleReconcile(t, lab, c, path, before)
			joined := strings.Join(problems, "\n")
			if !strings.Contains(joined, "duplicate offered key") || !strings.Contains(joined, "response and durable receipt differ") {
				t.Fatal("chunk boundary hid evidence corruption", problems)
			}
		})
	}
}

func TestScaleMixedPayloadsReconcileExactBytesAndOutputs(t *testing.T) {
	c := scaleConfig{Name: "mixed-reconcile", Rate: 20, Seconds: 1, PayloadMix: true}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	lab := newScaleLab(t, c)
	before := scaleAccounts(t, lab)
	path := t.TempDir() + "/ledger"
	r, err := BenchmarkPush(context.Background(), PushBenchmarkOptions{Producers: lab.producers, Client: lab.http.Client(), Rate: 20, Duration: 1000000000, Ledger: path, CompactLedger: true, PreplannedLedger: true, NoRetry: true})
	if err != nil || r.Accepted != 20 || r.AcceptedBytes != 1060000 || r.AcceptedObservations != 170 {
		t.Fatal(r, err)
	}
	if r.Payloads["webhook10k"].Accepted != 16 || r.Payloads["batch18_100k"].Accepted != 3 || r.Payloads["batch100_600k"].Accepted != 1 {
		t.Fatal(r.Payloads)
	}
	if resolved, problems := scaleReconcile(t, lab, c, path, before); resolved != 0 || len(problems) != 0 {
		t.Fatal(resolved, problems)
	}
}
