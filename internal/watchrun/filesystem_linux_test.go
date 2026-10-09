package watchrun_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestPhysicalFullPushIsNotAcknowledged(t *testing.T) {
	root := os.Getenv("DING_FS_FULL_DIR")
	if root == "" {
		t.Skip("requires dedicated qualification tmpfs")
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(root, &fs); err != nil || fs.Type != 0x01021994 || fs.Blocks*uint64(fs.Bsize) > 16<<20 {
		t.Fatal("requires dedicated bounded tmpfs", err)
	}
	dir, err := os.MkdirTemp(root, "push-")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a := watchrun.New(s)
	manifest := `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: push}
spec:
  source: {type: push}
  condition: {field: value, operator: gt, value: 1}
`
	if _, err := a.Apply(ctx, watchrun.ApplyRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a = watchrun.New(s)
	c := control.Credentials{Admin: strings.Repeat("a", 64), Ingest: strings.Repeat("i", 64)}
	server := httptest.NewServer(control.Handler(a, c))
	defer server.Close()
	filler := filepath.Join(root, "filler")
	f, err := os.Create(filler)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filler)
	var used int
	for used < 16<<20 {
		n, e := f.Write(make([]byte, 65536))
		used += n
		if e != nil {
			err = e
			break
		}
	}
	f.Close()
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatal("not physically full", err)
	}
	push := func() int {
		t.Helper()
		request, _ := http.NewRequest("POST", server.URL+"/v1/ingest/push", strings.NewReader(`{"value":2}`))
		request.Header.Set("Authorization", "Bearer "+c.Ingest)
		request.Header.Set("Idempotency-Key", "retry-me")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var data any
		if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode
	}
	if code := push(); code != 503 {
		t.Fatal("full filesystem acknowledged input", code)
	}
	if a.Health() == "" {
		t.Fatal("storage failure was not exposed")
	}
	var usage store.Usage
	if err := s.View(ctx, func(tx *store.Tx) error { var err error; usage, err = tx.Usage(); return err }); err != nil {
		t.Fatal(err)
	}
	if usage.Observations != 0 || usage.Events != 1 {
		t.Fatal("failed input changed durable state", usage)
	}
	if err := os.Remove(filler); err != nil {
		t.Fatal(err)
	}
	if code := push(); code != 202 {
		t.Fatal("failed to recover after freeing disk", code)
	}
	if a.Health() != "" {
		t.Fatal("successful durable input did not clear runtime storage error")
	}
	if code := push(); code != 202 {
		t.Fatal("dedup receipt failed", code)
	}
	if err := s.View(ctx, func(tx *store.Tx) error { var err error; usage, err = tx.Usage(); return err }); err != nil {
		t.Fatal(err)
	}
	if usage.Observations != 1 {
		t.Fatal("retried input duplicated observation", usage)
	}
	t.Log("physical ENOSPC returned 503 with no input commit; after freeing space the same key committed exactly once")
}
