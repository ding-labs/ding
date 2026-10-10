// Package cloudbackup produces encrypted, fenced offline backups. It is an
// operator capability and is never exposed through the tenant HTTP API.
package cloudbackup

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"filippo.io/age"
	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/store"
)

const Quarantine = "restore-required.json"
const maxFile = 256 << 20
const maxAccounts = 100

type manifest struct {
	Format    int       `json:"format"`
	CreatedAt time.Time `json:"createdAt"`
	Accounts  []string  `json:"accounts"`
}

func Write(ctx context.Context, dir, output, recipient string) error {
	rec, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		return fmt.Errorf("valid age X25519 recipient required")
	}
	// Holding the control lock rejects backups while a cloud worker is running.
	db, err := state.Open(ctx, dir)
	if err != nil {
		return fmt.Errorf("stop the cloud service before an offline backup: %w", err)
	}
	defer db.Close()
	accounts, err := db.Accounts(ctx)
	if err != nil {
		return err
	}
	if len(accounts) > maxAccounts {
		return fmt.Errorf("backup cohort limit exceeded")
	}
	stage, err := os.MkdirTemp(dir, ".backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	out, err := mcpconfig.CreatePrivate(output)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		out.Close()
		if !complete {
			_ = os.Remove(output)
		}
	}()
	encrypted, err := age.Encrypt(out, rec)
	if err != nil {
		return err
	}
	archive := tar.NewWriter(encrypted)
	m := manifest{Format: 1, CreatedAt: time.Now().UTC(), Accounts: []string{}}
	for _, a := range accounts {
		m.Accounts = append(m.Accounts, a.ID)
	}
	raw, _ := json.Marshal(m)
	if err := archive.WriteHeader(&tar.Header{Name: "format.json", Size: int64(len(raw)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err := archive.Write(raw); err != nil {
		return err
	}
	if err := db.Backup(ctx, filepath.Join(stage, "control.db")); err != nil {
		return err
	}
	if err := appendFile(ctx, archive, filepath.Join(stage, "control.db"), "control.db"); err != nil {
		return err
	}
	for _, a := range accounts {
		if err := ctx.Err(); err != nil {
			return err
		}
		tenant, err := store.Open(ctx, filepath.Join(dir, "workspaces", a.ID))
		if err != nil {
			return err
		}
		snapshot := filepath.Join(stage, a.ID+".db")
		err = tenant.Backup(ctx, snapshot)
		closed := tenant.Close()
		if err = errors.Join(err, closed); err != nil {
			return err
		}
		if err := appendFile(ctx, archive, snapshot, "workspaces/"+a.ID+"/ding.db"); err != nil {
			return err
		}
		if err := os.Remove(snapshot); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := encrypted.Close(); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}
func appendFile(ctx context.Context, w *tar.Writer, path, name string) error {
	f, err := mcpconfig.OpenPrivate(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > maxFile {
		return fmt.Errorf("database exceeds 256 MiB backup file limit")
	}
	if err := w.WriteHeader(&tar.Header{Name: name, Size: info.Size(), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err = io.Copy(w, contextReader{ctx, f})
	return err
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (c contextReader) Read(b []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(b)
}
