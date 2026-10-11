//go:build !windows

package logs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestSignalCaptureHelper(t *testing.T) {
	if os.Getenv("DING_SIGNAL_HELPER") != "1" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	fmt.Fprintln(os.Stdout, "INFO ready")
	<-signals
	fmt.Fprintln(os.Stderr, "INFO graceful termination")
	os.Exit(23)
}

type readyOutput struct {
	once  sync.Once
	ready chan struct{}
}

func (w *readyOutput) Write(b []byte) (int, error) {
	w.once.Do(func() { close(w.ready) })
	return len(b), nil
}

func TestExecForwardsTerminationAndKeepsChildExit(t *testing.T) {
	c, _ := NewCollector(ParserConfig{}, WindowConfig{Window: time.Minute, MinEvents: 1})
	defer c.Close()
	signals := make(chan os.Signal, 1)
	out := &readyOutput{ready: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan ExecResult, 1)
	go func() {
		done <- c.Exec(ctx, ExecOptions{Argv: []string{os.Args[0], "-test.run=^TestSignalCaptureHelper$"}, Env: append(os.Environ(), "DING_SIGNAL_HELPER=1"), Stdout: out, Signals: signals})
	}()
	select {
	case <-out.ready:
	case <-ctx.Done():
		t.Fatal("child did not start")
	}
	signals <- syscall.SIGTERM
	if r := <-done; r.ExitCode != 23 || r.Err != nil {
		t.Fatal(r)
	}
	c.Snapshot()
	s := c.window.Snapshot(time.Now().Add(time.Second))
	if s.Total != 2 || s.Stderr != 1 || s.Errors != 0 {
		t.Fatal(s)
	}
}

func TestExecBoundsDescendantHeldPipes(t *testing.T) {
	c, _ := NewCollector(ParserConfig{}, WindowConfig{Window: time.Minute, MinEvents: 1})
	defer c.Close()
	started := time.Now()
	r := c.Exec(context.Background(), ExecOptions{Argv: []string{"/bin/sh", "-c", "sleep 30 & exit 0"}})
	if time.Since(started) > 4*time.Second || r.ExitCode != 0 || r.Err == nil {
		t.Fatal(r, time.Since(started))
	}
	if c.Snapshot().Complete {
		t.Fatal("incomplete pipe drain not marked")
	}
}

type brokenOutput struct{}

func (brokenOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestForwardingFailureIsReportedWithoutInventingAnExitStatus(t *testing.T) {
	c, _ := NewCollector(ParserConfig{}, WindowConfig{Window: time.Minute, MinEvents: 1})
	defer c.Close()
	r := c.Exec(context.Background(), ExecOptions{Argv: []string{"/bin/sh", "-c", "printf 'WARN sample\\n'; exit 7"}, Stdout: brokenOutput{}})
	if r.ExitCode != 7 || !errors.Is(r.Err, io.ErrClosedPipe) {
		t.Fatal(r)
	}
}
