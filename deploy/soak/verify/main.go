// Verify an isolated, still-quarantined portable restore. Never starts a runner.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
)

func main() {
	if len(os.Args) != 5 {
		panic("usage: verify RESTORE_DIRECTORY DATA_KEY PROBE_JSON WATCH_ID")
	}
	ctx := context.Background()
	dir := os.Args[1]
	if _, err := os.Stat(filepath.Join(dir, "restore-required.json")); err != nil {
		panic("restore quarantine missing")
	}
	db, err := state.Open(ctx, dir)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	key, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	vault, err := state.NewVault(db, key)
	if err != nil {
		panic(err)
	}
	workspaces, err := db.Workspaces(ctx)
	if err != nil || len(workspaces) != 1 {
		panic("workspace inventory mismatch")
	}
	var probe struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	raw, err := os.ReadFile(os.Args[3])
	if err != nil || json.Unmarshal(raw, &probe) != nil {
		panic("probe unavailable")
	}
	got, err := vault.Get(ctx, workspaces[0].ID, probe.Name)
	if err != nil || got != probe.Value {
		panic("credential restoration failed")
	}
	tenant, err := store.Open(ctx, filepath.Join(dir, "workspaces", workspaces[0].ID))
	if err != nil {
		panic(err)
	}
	defer tenant.Close()
	err = tenant.View(ctx, func(tx *store.Tx) error {
		w, e := tx.Watch(os.Args[4])
		if e != nil {
			return e
		}
		if w.Status != "paused" {
			return fmt.Errorf("restored watch not paused")
		}
		n, e := tx.Pending()
		if e != nil {
			return e
		}
		if n != 0 {
			return fmt.Errorf("pending deliveries restored")
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("PASS: isolated restore remains quarantined, original data key decrypts the synthetic credential, watch paused and no pending deliveries")
}
