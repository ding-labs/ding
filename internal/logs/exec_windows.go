package logs

import (
	"os"
	"os/exec"
)

func setupCapture(cmd *exec.Cmd)                          {}
func signalCapture(cmd *exec.Cmd, signal os.Signal) error { return cmd.Process.Kill() }
func captureExitCode(state *os.ProcessState) int          { return state.ExitCode() }
