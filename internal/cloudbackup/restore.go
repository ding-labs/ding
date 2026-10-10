package cloudbackup

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"filippo.io/age"
	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/store"
)

var idPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func Restore(ctx context.Context, input, dir, identity string) error {
	key, err := age.ParseX25519Identity(identity)
	if err != nil {
		return fmt.Errorf("valid age X25519 identity required")
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("restore into a new absolute directory")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("restore directory must not exist: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(dir)
		}
	}()
	// This marker is present before any database exists and remains after success.
	marker, err := mcpconfig.CreatePrivate(filepath.Join(dir, Quarantine))
	if err != nil {
		return err
	}
	_, err = marker.WriteString(`{"status":"quarantined","reason":"verify all other runners are stopped; inspect canceled deliveries and reconnect clients"}`)
	closed := marker.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	in, err := os.Open(input)
	if err != nil {
		return err
	}
	defer in.Close()
	decrypted, err := age.Decrypt(contextReader{ctx, io.LimitReader(in, 32<<30)}, key)
	if err != nil {
		return fmt.Errorf("backup decryption failed")
	}
	archive := tar.NewReader(decrypted)
	first, err := archive.Next()
	if err != nil || first.Name != "format.json" || first.Typeflag != tar.TypeReg || first.Size > 32<<10 {
		return fmt.Errorf("invalid backup manifest")
	}
	raw, err := io.ReadAll(archive)
	if err != nil {
		return err
	}
	var m manifest
	if json.Unmarshal(raw, &m) != nil || m.Format != 1 || len(m.Accounts) > maxAccounts {
		return fmt.Errorf("unsupported backup manifest")
	}
	expected := map[string]bool{"control.db": false}
	for _, id := range m.Accounts {
		if !idPattern.MatchString(id) {
			return fmt.Errorf("invalid workspace in backup")
		}
		name := "workspaces/" + id + "/ding.db"
		if _, ok := expected[name]; ok {
			return fmt.Errorf("duplicate workspace")
		}
		expected[name] = false
	}
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("truncated backup: %w", err)
		}
		seen, ok := expected[header.Name]
		if !ok || seen || header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > maxFile {
			return fmt.Errorf("unexpected or oversized backup entry")
		}
		expected[header.Name] = true
		path := filepath.Join(dir, filepath.FromSlash(header.Name))
		f, err := mcpconfig.CreatePrivate(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, contextReader{ctx, archive})
		syncErr := f.Sync()
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	// tar EOF can precede the final authenticated age chunk. Always consume it.
	if _, err := io.Copy(io.Discard, contextReader{ctx, decrypted}); err != nil {
		return fmt.Errorf("backup authentication failed")
	}
	for _, seen := range expected {
		if !seen {
			return fmt.Errorf("backup is missing a database")
		}
	}
	db, err := state.Open(ctx, dir)
	if err != nil {
		return err
	}
	accounts, err := db.Accounts(ctx)
	if err == nil {
		ids := []string{}
		for _, a := range accounts {
			ids = append(ids, a.ID)
		}
		slices.Sort(ids)
		slices.Sort(m.Accounts)
		if !slices.Equal(ids, m.Accounts) {
			err = fmt.Errorf("backup workspace inventory mismatch")
		}
	}
	if err == nil {
		err = db.QuarantineRestore(ctx)
	}
	closed = db.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	for _, id := range m.Accounts {
		tenant, err := store.Open(ctx, filepath.Join(dir, "workspaces", id))
		if err != nil {
			return err
		}
		err = tenant.QuarantineRestore(ctx)
		closed = tenant.Close()
		if err != nil {
			return err
		}
		if closed != nil {
			return closed
		}
	}
	complete = true
	return nil
}

// Release is an operator's explicit declaration that restored ownership has been
// reviewed. It never resumes watches or retries canceled external deliveries.
func Release(ctx context.Context, dir string) error {
	if err := mcpconfig.CheckPrivate(filepath.Join(dir, Quarantine)); err != nil {
		return err
	}
	db, err := state.Open(ctx, dir)
	if err != nil {
		return err
	}
	defer db.Close()
	accounts, err := db.Accounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if !idPattern.MatchString(a.ID) {
			return fmt.Errorf("invalid workspace")
		}
		tenant, err := store.Open(ctx, filepath.Join(dir, "workspaces", a.ID))
		if err != nil {
			return err
		}
		err = tenant.ReleaseRestoredHolds(ctx)
		closed := tenant.Close()
		if err != nil {
			return err
		}
		if closed != nil {
			return closed
		}
	}
	return os.Remove(filepath.Join(dir, Quarantine))
}

func ReadIdentity(path string) (string, error) {
	f, err := mcpconfig.OpenPrivate(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(data) > 4096 {
		return "", fmt.Errorf("invalid private identity file")
	}
	return strings.TrimSpace(string(data)), nil
}
