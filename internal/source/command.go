package source

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/transform"
	"github.com/ding-labs/ding/internal/watch"
)

type Command struct{ Lookup Lookup }

func (c Command) Fetch(ctx context.Context, p plan.Compiled, cursor string, now time.Time) Batch {
	unknown := func(reason string) Batch {
		return Batch{Cursor: cursor, Observations: []watch.Observation{{Health: "unknown", Detail: reason}}}
	}
	spec := p.Definition.Spec.Source
	if len(spec.Argv) == 0 || spec.Directory == "" {
		return unknown("command_invalid")
	}
	timeout, _ := time.ParseDuration(spec.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.Directory
	cmd.Env = []string{}
	names := make([]string, 0, len(spec.Env))
	for name := range spec.Env {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ref := spec.Env[name]
		value, err := Resolve(&ref, c.Lookup)
		if err != nil {
			return unknown("missing_credentials")
		}
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	captured := &capture{limit: p.Definition.Spec.Limits.MaxBytes, cancel: cancel}
	cmd.Stdout = outputStream{captured, true}
	cmd.Stderr = outputStream{captured, false}
	cmd.WaitDelay = 250 * time.Millisecond
	started, cleanup, err := prepareTree(cmd)
	if err != nil {
		return unknown("command_process_setup_failed")
	}
	defer cleanup()
	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return unknown("command_timeout")
		}
		return unknown("command_start_failed")
	}
	if err := started(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if ctx.Err() != nil {
			return unknown("command_timeout")
		}
		return unknown("command_process_setup_failed")
	}
	err = cmd.Wait()
	if captured.exceeded {
		return unknown("command_output_limit")
	}
	if ctx.Err() != nil {
		return unknown("command_timeout")
	}
	if err != nil {
		return unknown("command_failed")
	}
	outputs, err := transform.Project(ctx, captured.stdout.Bytes(), spec.JQ, spec.Fields, p.Definition.Spec.Limits.MaxOutputs, p.Definition.Spec.Limits.MaxBytes)
	if err != nil {
		return unknown("command_invalid_projection")
	}
	batch := Batch{Cursor: cursor}
	batch.Observations, err = Observations(outputs, spec.ObservedAtField)
	if err != nil {
		return unknown("command_invalid_observed_time")
	}

	return batch
}

type capture struct {
	mu          sync.Mutex
	stdout      bytes.Buffer
	used, limit int
	exceeded    bool
	cancel      context.CancelFunc
}
type outputStream struct {
	capture *capture
	keep    bool
}

func (w outputStream) Write(p []byte) (int, error) {
	c := w.capture
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used+len(p) > c.limit {
		c.exceeded = true
		c.cancel()
		return 0, fmt.Errorf("command output exceeds limit")
	}
	c.used += len(p)
	if w.keep {
		return c.stdout.Write(p)
	}
	return len(p), nil
}
