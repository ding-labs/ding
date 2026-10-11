//go:build !windows

package logs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileFollowsCompleteRecordsAndMarksReplacementGaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	os.WriteFile(path, []byte("WARN historical\n"), 0600)
	c, _ := NewCollector(ParserConfig{}, WindowConfig{Window: time.Minute, MinEvents: 1})
	defer c.Close()
	f := &FileFollower{Path: path, Collector: c}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.Run(ctx) }()
	defer func() { cancel(); <-done }()
	wait := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !check() {
			if time.Now().After(deadline) {
				t.Fatal("file follower did not advance", f.Position())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait(func() bool { return f.Position().Identity != "" })
	original := f.Position()
	appendText := func(s string) {
		t.Helper()
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := file.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	appendText("WARN live\nINFO partial")
	wait(func() bool { return f.Position().Offset == original.Offset+10 })
	appendText("\n")
	wait(func() bool { return f.Position().Offset == original.Offset+23 })
	c.Snapshot()
	s := c.window.Snapshot(time.Now().Add(time.Second))
	if s.Total != 2 || s.Warnings != 1 {
		t.Fatal(s)
	}
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("INFO replacement backlog\n"), 0600)
	wait(func() bool { return f.Position().Identity != original.Identity })
	if c.window.Snapshot(time.Now().Add(time.Second)).Complete {
		t.Fatal("rotation not marked as gap")
	}
	os.WriteFile(path, []byte("WARN truncated\n"), 0600)
	wait(func() bool { return f.Position().Offset == 15 && f.Position().SkippedFrom != 0 })
	appendText("INFO new\n")
	wait(func() bool { return f.Position().Offset == 24 })
	c.Snapshot()
	if s := c.window.Snapshot(time.Now().Add(time.Second)); s.Total != 3 {
		t.Fatal(s)
	}
}

func TestFileRejectsNonRegularInput(t *testing.T) {
	f := &FileFollower{Path: t.TempDir()}
	if err := f.Run(context.Background()); err != ErrFileRead {
		t.Fatal(err)
	}
}
