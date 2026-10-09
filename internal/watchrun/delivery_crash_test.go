package watchrun

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
)

func TestReceiverAcceptsThenProcessDiesBeforeAcknowledgment(t *testing.T) {
	if os.Getenv("DING_DELIVERY_CRASH_CHILD") == "1" {
		s, err := store.Open(ctx, os.Getenv("DING_DELIVERY_CRASH_DIR"))
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		a := New(s)
		a.Now = func() time.Time { return start.Add(20 * time.Second) }
		a.Lookup = func(string) (string, bool) { return os.Getenv("DING_DELIVERY_CRASH_URL"), true }
		if _, err := a.DeliverOne(ctx); err != nil {
			t.Fatal(err)
		}
		return
	}
	accepted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var mu sync.Mutex
	var ids []string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		mu.Lock()
		ids = append(ids, r.Header.Get("X-Ding-Event-Id"))
		first := len(ids) == 1
		mu.Unlock()
		if first {
			close(accepted)
			<-release
		}
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	defer unblock()
	a, dir := setup(t)
	r := apply(t, a, "https://example.com")
	for n := 1; n <= 3; n++ {
		input(t, a, r, n, 500)
	}
	a.Store.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestReceiverAcceptsThenProcessDiesBeforeAcknowledgment$")
	command.Env = append(os.Environ(), "DING_DELIVERY_CRASH_CHILD=1", "DING_DELIVERY_CRASH_DIR="+dir, "DING_DELIVERY_CRASH_URL="+receiver.URL)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-accepted:
	case <-time.After(10 * time.Second):
		command.Process.Kill()
		command.Wait()
		unblock()
		t.Fatal("child did not deliver", output.String())
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	unblock()
	reopened, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	a = New(reopened)
	a.Now = func() time.Time { return start.Add(51 * time.Second) }
	a.Lookup = func(string) (string, bool) { return receiver.URL, true }
	if worked, err := a.DeliverOne(ctx); err != nil || !worked {
		t.Fatal(worked, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ids) != 2 || ids[0] == "" || ids[0] != ids[1] {
		t.Fatal("retry changed stable event identity", ids)
	}
	inspection, err := a.Inspect(ctx, "api")
	if err != nil || len(inspection.Deliveries) != 1 || inspection.Deliveries[0].Status != "delivered" || inspection.Deliveries[0].Attempts != 2 {
		t.Fatal(inspection, err)
	}
}
func TestCredentialRotationUsesPinnedReferenceOnManualRetry(t *testing.T) {
	var calls atomic.Int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer receiver.Close()
	a, _ := setup(t)
	r := apply(t, a, "https://example.com")
	for n := 1; n <= 3; n++ {
		input(t, a, r, n, 500)
	}
	a.Now = func() time.Time { return start.Add(20 * time.Second) }
	a.Lookup = func(string) (string, bool) { return "", false }
	if _, err := a.DeliverOne(ctx); err != nil {
		t.Fatal(err)
	}
	before, _ := a.Inspect(ctx, "api")
	if before.Deliveries[0].Status != "permanent" {
		t.Fatal(before)
	}
	a.Lookup = func(string) (string, bool) { return receiver.URL, true }
	if err := a.Retry(ctx, before.Deliveries[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DeliverOne(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := a.Inspect(ctx, "api")
	if calls.Load() != 1 || after.Deliveries[0].Status != "delivered" || before.Deliveries[0].EventID != after.Deliveries[0].EventID || before.Deliveries[0].DestinationRevision != after.Deliveries[0].DestinationRevision {
		t.Fatal(before, after, calls.Load())
	}
}
