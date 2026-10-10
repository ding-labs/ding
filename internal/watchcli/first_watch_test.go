package watchcli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestCreateWatchPreviewsBeforeExplicitActivation(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c, err := control.PrivateCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}
	app := watchrun.New(db)
	server := httptest.NewServer(control.ConsoleHandler(app, c, control.ConsoleConfig{}))
	defer server.Close()
	if err := control.SaveConnection(dir, server.URL); err != nil {
		t.Fatal(err)
	}
	args := []string{"watch", "create", "http://127.0.0.1:3000/health", "--delivery", "console", "--state-dir", dir}
	var out bytes.Buffer
	if err := Execute("dev", args, &out, &out); err != nil {
		t.Fatal(err, out.String())
	}
	rows, err := app.List(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("preview created watch", err)
	}
	if err := Execute("dev", append(args, "--yes"), &out, &out); err != nil {
		t.Fatal(err, out.String())
	}
	rows, err = app.List(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatal("activation did not create watch", err)
	}
	if err := Execute("dev", append(args, "--yes"), &out, &out); err == nil {
		t.Fatal("overwrote an existing watch")
	}
}
