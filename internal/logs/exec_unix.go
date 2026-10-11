//go:build !windows

package logs

import (
	"os"
	"os/exec"
	"syscall"
)

func setupCapture(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalCapture(cmd *exec.Cmd, signal os.Signal) error {
	s, ok := signal.(syscall.Signal)
	if !ok {
		return os.ErrInvalid
	}
	return syscall.Kill(-cmd.Process.Pid, s)
}

func captureExitCode(state *os.ProcessState) int {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return state.ExitCode()
}
