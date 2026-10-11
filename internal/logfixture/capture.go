package logfixture

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ding-labs/ding/internal/logs"
)

type CaptureOptions struct {
	Duration               time.Duration
	Rate, Size             int
	Format, Transport, Out string
}

func Capture(ctx context.Context, o CaptureOptions, c *logs.Collector, out io.Writer) error {
	program, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"--producer-report", filepath.Join(o.Out, "producer.json"), "--duration", o.Duration.String(), "--rate", strconv.Itoa(o.Rate), "--bytes", strconv.Itoa(o.Size), "--format", o.Format}
	if o.Transport == "exec" {
		r := c.Exec(ctx, logs.ExecOptions{Argv: append([]string{program}, args...), Stdout: out, Stderr: out})
		return r.Err
	}
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.WaitDelay = 2 * time.Second
	if o.Transport == "pipe" {
		read, write, err := os.Pipe()
		if err != nil {
			return err
		}
		defer read.Close()
		defer write.Close()
		cmd.Stdout, cmd.Stderr = write, write
		if err := cmd.Start(); err != nil {
			return err
		}
		write.Close()
		frame := c.Stream()
		_, copyErr := io.Copy(io.MultiWriter(out, frame), read)
		frame.Close()
		if err := cmd.Wait(); err != nil {
			return err
		}
		return copyErr
	}
	path := filepath.Join(o.Out, "application.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	followCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var followErr error
	follower := &logs.FileFollower{Path: path, Collector: c}
	go func() { followErr = follower.Run(followCtx); close(done) }()
	defer func() { cancel(); <-done }()
	for follower.Position().Identity == "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return followErr
		case <-time.After(10 * time.Millisecond):
		}
	}
	cmd.Stdout, cmd.Stderr = io.MultiWriter(file, out), io.MultiWriter(file, out)
	if err := cmd.Run(); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for follower.Position().Offset < info.Size() {
		if time.Now().After(deadline) {
			return io.ErrNoProgress
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}
