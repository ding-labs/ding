package cloud

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/store"
)

func TestOfflineDeletionRequiresStoppedOwnerAndKeepsOtherAccounts(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	db, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := db.Enroll(ctx, "issuer", "a", 2)
	b, _ := db.Enroll(ctx, "issuer", "b", 2)
	workspace := filepath.Join(dir, "workspaces", a.ID)
	data, err := store.Open(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	if err := DeleteOffline(ctx, dir, a.ID); err == nil {
		t.Fatal("deleted under a live owner")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := DeleteOffline(ctx, dir, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatal("workspace files survived")
	}
	db, err = state.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Account(ctx, a.ID); err == nil {
		t.Fatal("deleted account survived")
	}
	if _, err := db.Account(ctx, b.ID); err != nil {
		t.Fatal("other account removed", err)
	}
}
