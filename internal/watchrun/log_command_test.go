//go:build !windows

package watchrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLogReaderHelper(t *testing.T) {
	if os.Getenv("DING_LOG_READER_HELPER") != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv("DING_PID_FILE"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		os.Exit(1)
	}
	for {
		fmt.Fprintln(os.Stdout, "WARN observed")
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPauseStopsOwnedObserverAndLeavesIndependentApplication(t *testing.T) {
	application := exec.Command("/bin/sleep", "30")
	if err := application.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { application.Process.Kill(); application.Wait() }()
	a, _ := setup(t)
	a.Now = time.Now
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "reader.pid")
	a.Lookup = func(name string) (string, bool) {
		switch name {
		case "HELPER":
			return "1", true
		case "PID_FILE":
			return pidFile, true
		}
		return "", false
	}
	input := fmt.Sprintf(`input: {type: command, argv: [%q, "-test.run=^TestLogReaderHelper$"], directory: %q, env: {DING_LOG_READER_HELPER: {env: HELPER}, DING_PID_FILE: {env: PID_FILE}}}, window: 1s`, os.Args[0], dir)
	manifest := strings.Replace(nativeLogManifest, "window: 1s", input, 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- a.Run(runCtx) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(8 * time.Second)
	var pid int
	for pid == 0 {
		data, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(string(data))
		if time.Now().After(deadline) {
			t.Fatal("observer did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := a.Lifecycle(ctx, "output", LifecycleRequest{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatal("observer survived pause")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(application.Process.Pid, 0); err != nil {
		t.Fatal("independent application was stopped", err)
	}
}
