// Package mcpbridge routes explicit local and cloud connections without sharing
// an active target between conversations or borrowing administrator credentials.
package mcpbridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
)

type binding struct {
	Target mcpcontract.Target `json:"target"`
	Digest string             `json:"digest"`
}

// bind records the target before a mutation is sent. A lost response or adapter
// restart cannot reinterpret its operation key against a different connection.
// The daemon's receipt remains the authority for the operation's outcome.
func bind(dir, key, name string, args map[string]any, target mcpcontract.Target, write bool) error {
	if len(key) < 1 || len(key) > 256 {
		return mcpclient.Fail("invalid_operation", "use the original operation key")
	}
	unlock, err := mcpconfig.Lock(filepath.Join(dir, "journal.lock"))
	if err != nil {
		return mcpclient.Fail("connection_busy", "another operation is updating the target journal; retry this same request")
	}
	defer unlock()
	hash := sha256.Sum256([]byte(key))
	path := filepath.Join(dir, hex.EncodeToString(hash[:])+".json")
	raw, err := json.Marshal([]any{name, args})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	next := binding{target, hex.EncodeToString(digest[:])}
	var old binding
	err = mcpconfig.ReadPrivateJSON(path, &old, true)
	if err == nil {
		if old.Target != target || (write && old.Digest != next.Digest) {
			return mcpclient.Fail("operation_conflict", "this operation key belongs to another target or request; reconcile it on its original connection")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return mcpclient.Fail("journal_unavailable", "cannot read the saved operation target")
	}
	if !write {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 10000 {
		return mcpclient.Fail("journal_full", "the bridge operation journal is full; review receipts before removing old records")
	}
	return mcpconfig.AtomicJSON(path, next)
}
