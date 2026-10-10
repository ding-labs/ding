package cloud

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestTenantEnginesRemainIsolatedAndResumeAfterRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := state.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := make([]byte, 32)
	rand.Read(key)
	vault, _ := state.NewVault(db, key)
	a, _ := db.Enroll(ctx, "issuer", "a", 2)
	b, _ := db.Enroll(ctx, "issuer", "b", 2)
	client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("healthy"))}, nil
	})}
	p := NewPool(ctx, root, db, vault, client)
	first, err := p.Get(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Get(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.App.Apply(ctx, watchrun.ApplyRequest{Manifest: fixture}); err != nil {
		t.Fatal(err)
	}
	if records, err := second.App.List(ctx); err != nil || len(records) != 0 {
		t.Fatal("watch crossed workspace", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	observed := false
	for time.Now().Before(deadline) {
		records, err := first.App.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) == 1 && !records[0].LastInputAt.IsZero() {
			observed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !observed {
		t.Fatal("hosted engine never acquired")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	p = NewPool(ctx, root, db, vault, client)
	defer p.Close()
	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	first, err = p.Get(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	records, err := first.App.List(ctx)
	if err != nil || len(records) != 1 || records[0].LastInputAt.IsZero() {
		t.Fatal("restart lost hosted evidence", err)
	}
	if _, err := p.Get(ctx, "../../another-workspace"); err == nil {
		t.Fatal("path traversal accepted")
	}
}
