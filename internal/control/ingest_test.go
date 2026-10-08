package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

const pushManifest = `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: events}
spec:
  source: {type: push, jq: '.events[]'}
  condition: {field: id, operator: new-event, dedupFor: 1h}
  policy: {trigger: level, interval: 0s}
  limits: {maxBytes: 1024, maxOutputs: 2}
`

func TestPushAuthenticationAtomicityAndDedup(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := watchrun.New(s)
	app.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	if _, err := app.Apply(ctx, watchrun.ApplyRequest{Manifest: pushManifest}); err != nil {
		t.Fatal(err)
	}
	c := Credentials{Admin: strings.Repeat("a", 64), Ingest: strings.Repeat("i", 64)}
	server := httptest.NewServer(Handler(app, c))
	defer server.Close()
	post := func(token, id, body string) (int, watchrun.Receipt) {
		t.Helper()
		request, _ := http.NewRequest("POST", server.URL+"/v1/ingest/events", bytes.NewBufferString(body))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Idempotency-Key", id)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result struct {
			Data watchrun.Receipt `json:"data"`
		}
		data, _ := io.ReadAll(response.Body)
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(string(data), err)
		}
		return response.StatusCode, result.Data
	}
	payload := `{"events":[{"id":"a"},{"id":"b"}]}`
	for _, token := range []string{"", c.Admin, "wrong"} {
		if status, _ := post(token, "one", payload); status != 401 {
			t.Fatal("ingest role not enforced", status)
		}
	}
	status, first := post(c.Ingest, "one", payload)
	if status != 202 || first.Last-first.First != 1 {
		t.Fatal(status, first)
	}
	status, again := post(c.Ingest, "one", payload)
	if status != 202 || !again.Duplicate || again.First != first.First {
		t.Fatal(status, again)
	}
	if status, _ := post(c.Ingest, "two", payload); status != 202 {
		t.Fatal(status)
	} // provider IDs suppress events across distinct input requests
	var usage store.Usage
	s.View(ctx, func(tx *store.Tx) error { usage, _ = tx.Usage(); return nil })
	if usage.Observations != 4 || usage.Events != 3 {
		t.Fatal("provider dedup failed", usage)
	} // applied + two events
	for _, tc := range []struct {
		body   string
		status int
	}{{`{`, 400}, {`{"events":[{"id":"c"},{"id":{}}]}`, 400}, {`{"events":[{}, {}, {}]}`, 400}, {strings.Repeat("x", 1025), 413}} {
		if status, _ := post(c.Ingest, "bad", tc.body); status != tc.status {
			t.Fatal(tc, status)
		}
	}
	var after store.Usage
	s.View(ctx, func(tx *store.Tx) error { after, _ = tx.Usage(); return nil })
	if after.Observations != usage.Observations {
		t.Fatal("invalid batch partially accepted")
	}
	for n := 0; n < app.AcquisitionWorkers; n++ {
		if err := app.Reserve(); err != nil {
			t.Fatal(err)
		}
	}
	if status, _ := post(c.Ingest, "busy", payload); status != 503 {
		t.Fatal("unbounded push work", status)
	}
	for n := 0; n < app.AcquisitionWorkers; n++ {
		app.Release()
	}
	var wg sync.WaitGroup
	receipts := make(chan watchrun.Receipt, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, r := post(c.Ingest, "concurrent", `{"events":[{"id":"c"}]}`)
			if status != 202 {
				t.Error(status)
			}
			receipts <- r
		}()
	}
	wg.Wait()
	close(receipts)
	var sequence int64
	fresh := 0
	for r := range receipts {
		if sequence == 0 {
			sequence = r.First
		}
		if sequence != r.First {
			t.Fatal("duplicate committed twice")
		}
		if !r.Duplicate {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("wrong fresh count", fresh)
	}
	if _, err := app.Lifecycle(ctx, "events", watchrun.LifecycleRequest{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	if status, _ := post(c.Ingest, "paused", payload); status != 409 {
		t.Fatal(status)
	}
}
