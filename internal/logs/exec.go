package logs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type ExecOptions struct {
	Argv           []string
	Directory      string
	Env            []string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Signals        <-chan os.Signal
}

type ExecResult struct {
	ExitCode int
	Err      error
}

type forwarding struct {
	out    io.Writer
	frame  *Framer
	failed func(error)
}

func (f forwarding) Write(p []byte) (int, error) {
	n, err := f.out.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	// Analyze exactly the bytes that reached the original destination.
	f.frame.Write(p[:n])
	if err != nil {
		f.failed(err)
	}
	return n, err
}

// Exec owns only the explicitly launched process. Analytics errors cannot change
// its exit status. A bounded wait handles descendants retaining output handles.
func (c *Collector) Exec(ctx context.Context, opts ExecOptions) ExecResult {
	if len(opts.Argv) == 0 || opts.Argv[0] == "" {
		return ExecResult{ExitCode: 1, Err: fmt.Errorf("an application command is required")}
	}
	if err := ctx.Err(); err != nil {
		return ExecResult{ExitCode: 1, Err: err}
	}
	cmd := exec.Command(opts.Argv[0], opts.Argv[1:]...)
	cmd.Dir, cmd.Env, cmd.Stdin = opts.Directory, opts.Env, opts.Stdin
	a, b := c.Stream("stdout"), c.Stream("stderr")
	defer a.Close()
	defer b.Close()
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	var captureError error
	var failedOnce sync.Once
	failed := func(err error) { c.Gap(); failedOnce.Do(func() { captureError = err }) }
	cmd.Stdout, cmd.Stderr = forwarding{opts.Stdout, a, failed}, forwarding{opts.Stderr, b, failed}
	cmd.WaitDelay = 2 * time.Second
	setupCapture(cmd)
	if err := cmd.Start(); err != nil {
		return ExecResult{ExitCode: 1, Err: fmt.Errorf("cannot start application: %w", err)}
	}
	defer signalCapture(cmd, os.Kill)
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var kill <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	armKill := func() {
		if timer == nil {
			timer = time.NewTimer(5 * time.Second)
			kill = timer.C
		} else {
			_ = signalCapture(cmd, os.Kill)
		}
	}
	done, signals := ctx.Done(), opts.Signals
	for {
		select {
		case err := <-wait:
			code := captureExitCode(cmd.ProcessState)
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				err = nil // A nonzero child status is a result, not a capture failure.
			}
			err = errors.Join(err, captureError)
			if err != nil {
				c.Gap()
			}
			return ExecResult{ExitCode: code, Err: err}
		case s, ok := <-signals:
			if !ok {
				signals = nil
				continue
			}
			_ = signalCapture(cmd, s)
			armKill()
		case <-done:
			_ = signalCapture(cmd, os.Interrupt)
			armKill()
			done = nil
		case <-kill:
			_ = signalCapture(cmd, os.Kill)
			kill = nil
		}
	}
}
