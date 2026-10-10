package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

type Hooks struct {
	Backup func(context.Context, string) error
	Stop   func(context.Context) error
	Start  func(context.Context) error // Must wait for authenticated readiness.
}

type asset struct{ Target, Candidate, Previous string }
type journal struct {
	Phase         string
	Stage         string
	Before, After install.Record
	Assets        []asset
}

func journalPath(dir string) string { return filepath.Join(dir, "update.json") }

// Apply requires the installation lock, a verified/staged release, and a running
// managed service. A retained SQLite backup is created before stopping execution.
// Only equal-schema updates are accepted by Manifest.Select at the entry point.
func Apply(ctx context.Context, before install.Record, version, stage string, hooks Hooks) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Windows updates require the signed installer; an executing EXE cannot be replaced safely here")
	}
	if before.Owner != "standalone" {
		return fmt.Errorf("update this %s installation with its package manager", before.Owner)
	}
	if hooks.Backup == nil || hooks.Stop == nil || hooks.Start == nil {
		return fmt.Errorf("update lifecycle hooks are required")
	}
	if _, err := os.Lstat(journalPath(before.StateDir)); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("an update journal already exists; run ding update recover")
	}
	current, err := install.Inspect(before.Executable, before.StateDir, before.Version, before.Owner)
	if err != nil || current.Digest != before.Digest {
		return fmt.Errorf("installed executable changed outside Ding; repair ownership before updating")
	}
	stage, err = filepath.Abs(stage)
	if err != nil {
		return err
	}
	after, err := install.Inspect(filepath.Join(stage, "ding"), before.StateDir, version, before.Owner)
	if err != nil {
		return err
	}
	after.Executable, after.CreatedAt = before.Executable, before.CreatedAt
	j := journal{Phase: "prepared", Stage: stage, Before: before, After: after}
	j.Assets = append(j.Assets, asset{before.Executable, filepath.Join(stage, "ding"), filepath.Join(stage, "previous-0")})
	if runtime.GOOS == "darwin" {
		j.Assets = append(j.Assets, asset{filepath.Join(filepath.Dir(before.Executable), "DingNotifications.app"), filepath.Join(stage, "DingNotifications.app"), filepath.Join(stage, "previous-1")})
	}
	if err = j.validate(before.StateDir); err != nil {
		return err
	}
	for _, a := range j.Assets {
		info, err := os.Lstat(a.Target)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("update target is missing or linked; use a complete packaged installation")
		}
		if _, err := os.Lstat(a.Candidate); err != nil {
			return err
		}
	}
	backup := filepath.Join(before.StateDir, "backups", filepath.Base(stage)+".db")
	if err = os.MkdirAll(filepath.Dir(backup), 0700); err != nil {
		return err
	}
	if err = hooks.Backup(ctx, backup); err != nil {
		return fmt.Errorf("pre-update backup failed: %w", err)
	}
	if err = install.AtomicJSON(journalPath(before.StateDir), j); err != nil {
		return err
	}
	applyErr := func() error {
		if err := hooks.Stop(ctx); err != nil {
			return err
		}
		for _, a := range j.Assets {
			if _, err := os.Lstat(a.Target); err == nil {
				if err = os.Rename(a.Target, a.Previous); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.Rename(a.Candidate, a.Target); err != nil {
				return err
			}
			if err := syncAssets(j.Stage, filepath.Dir(a.Target)); err != nil {
				return err
			}
		}
		if err := install.Replace(after); err != nil {
			return err
		}
		if err := hooks.Start(ctx); err != nil {
			return err
		}
		j.Phase = "committed"
		return install.AtomicJSON(journalPath(before.StateDir), j)
	}()
	if applyErr != nil {
		// The caller's context may already be canceled; recovery gets a fresh bound.
		recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
		defer cancel()
		if err := Recover(recoveryCtx, before.StateDir, hooks); err != nil {
			return errors.Join(applyErr, fmt.Errorf("recovery incomplete; run ding update recover: %w", err))
		}
		return fmt.Errorf("update failed and the previous version was restored: %w", applyErr)
	}
	return finish(j)
}

func (j journal) validate(dir string) error {
	if err := j.Before.Validate(); err != nil {
		return err
	}
	if err := j.After.Validate(); err != nil {
		return err
	}
	if j.Before.StateDir != dir || j.After.StateDir != dir || j.Before.Executable != j.After.Executable || j.Before.Owner != "standalone" || j.After.Owner != "standalone" {
		return fmt.Errorf("invalid update ownership")
	}
	root := filepath.Dir(j.Before.Executable)
	if filepath.Dir(j.Stage) != root || !strings.HasPrefix(filepath.Base(j.Stage), ".ding-update-") || len(j.Assets) < 1 || len(j.Assets) > 2 {
		return fmt.Errorf("invalid update staging path")
	}
	for i, a := range j.Assets {
		target, name := j.Before.Executable, "ding"
		if i == 1 {
			target, name = filepath.Join(root, "DingNotifications.app"), "DingNotifications.app"
		}
		if a.Target != target || a.Candidate != filepath.Join(j.Stage, name) || a.Previous != filepath.Join(j.Stage, fmt.Sprintf("previous-%d", i)) {
			return fmt.Errorf("invalid update asset paths")
		}
	}
	if j.Phase != "prepared" && j.Phase != "committed" {
		return fmt.Errorf("unknown update phase")
	}
	return nil
}

// Recover is repeatable after interruption. Never restore the database from the
// backup automatically: this transaction only permits equal-schema binaries.
func Recover(ctx context.Context, dir string, hooks Hooks) error {
	var j journal
	if err := mcpconfig.ReadPrivateJSON(journalPath(dir), &j, true); err != nil {
		return err
	}
	if err := j.validate(dir); err != nil {
		return err
	}
	if j.Phase == "committed" {
		return finish(j)
	}
	if hooks.Stop == nil || hooks.Start == nil {
		return fmt.Errorf("recovery lifecycle hooks are required")
	}
	if err := hooks.Stop(ctx); err != nil {
		return err
	}
	for _, a := range j.Assets {
		if _, err := os.Lstat(a.Previous); err == nil {
			if err := os.RemoveAll(a.Target); err != nil {
				return err
			}
			if err := os.Rename(a.Previous, a.Target); err != nil {
				return err
			}
			if err := syncAssets(j.Stage, filepath.Dir(a.Target)); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := install.Replace(j.Before); err != nil {
		return err
	}
	if err := hooks.Start(ctx); err != nil {
		return err
	}
	return finish(j)
}

func finish(j journal) error {
	// Remove the journal first: a crash during cleanup cannot trigger rollback.
	if err := os.Remove(journalPath(j.Before.StateDir)); err != nil {
		return err
	}
	return os.RemoveAll(j.Stage)
}
