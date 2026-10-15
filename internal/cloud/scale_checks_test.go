package cloud

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/condition"
)

func scaleSend(t *testing.T, lab *scaleLab, p PushProducer, key string, body []byte) (int, []byte) {
	t.Helper()
	for attempt := 0; attempt < 5; attempt++ {
		r, err := http.NewRequest("POST", p.URL, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+os.Getenv(p.TokenEnv))
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("Content-Type", "application/json")
		response, err := lab.http.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 429 {
			return response.StatusCode, data
		}
		time.Sleep(time.Second)
	}
	return 429, nil
}

func scaleChecks(t *testing.T, lab *scaleLab, c scaleConfig, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var ack pushLedgerEntry
	for scanner.Scan() {
		var e pushLedgerEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if e.Receipt != nil {
			ack = e
			break
		}
	}
	if ack.Receipt == nil {
		return nil
	} // A fully rejected capacity case has no acknowledged input.
	before := scaleAccounts(t, lab)
	p := lab.producers[ack.Producer]
	status, data := scaleSend(t, lab, p, ack.Key, p.Body)
	var replay struct {
		Data struct {
			AcceptanceID string `json:"acceptanceId"`
			Duplicate    bool   `json:"duplicate"`
		}
	}
	if status != 202 || json.Unmarshal(data, &replay) != nil || !replay.Data.Duplicate || replay.Data.AcceptanceID != ack.Receipt.AcceptanceID {
		return []string{"HTTP replay identity failed"}
	}
	changed := bytes.Replace(p.Body, []byte(`"value":50`), []byte(`"value":51`), 1)
	if bytes.Equal(changed, p.Body) {
		changed = bytes.Replace(p.Body, []byte(`"value":100`), []byte(`"value":101`), 1)
	}
	if bytes.Equal(changed, p.Body) {
		t.Fatal("fixture has no numeric value")
	}
	status, _ = scaleSend(t, lab, p, ack.Key, changed)
	if status != 409 {
		return []string{"changed-byte HTTP conflict failed"}
	}
	after := scaleAccounts(t, lab)
	for i := range before {
		if before[i].Usage != after[i].Usage {
			return []string{"replay/conflict changed durable usage"}
		}
	}
	if c.Condition == "alerts" {
		for i, p := range lab.producers {
			var body map[string]any
			if err := json.Unmarshal(p.Body, &body); err != nil {
				t.Fatal(err)
			}
			for _, e := range body["events"].([]any) {
				e.(map[string]any)["value"] = 50
			}
			raw, _ := json.Marshal(body)
			status, _ := scaleSend(t, lab, p, fmt.Sprintf("final-recovery-%d", i), raw)
			if status != 202 {
				return []string{"final healthy input rejected"}
			}
		}
	}
	for _, tenant := range lab.tenants {
		inspection, err := tenant.App.Inspect(context.Background(), "ci")
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range inspection.Entities {
			var state condition.State
			if err := json.Unmarshal(raw, &state); err != nil {
				t.Fatal(err)
			}
			if state.Open || state.SourceUnhealthy {
				return []string{"final group not healthy"}
			}
		}
	}
	return nil
}
