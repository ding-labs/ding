//go:build darwin || linux

package watchrun

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

// Opt-in because two 30-second observations are unsuitable for routine tests.
// Run alone, without -race or CPU profiling, to measure actual process CPU.
func TestIdleCPUUsage(t *testing.T) {
	if os.Getenv("DING_MEASURE_IDLE_CPU") != "1" {
		t.Skip("set DING_MEASURE_IDLE_CPU=1 for idle CPU measurements")
	}
	for _, paused := range []bool{false, true} {
		name := "empty"
		if paused {
			name = "paused-watch"
		}
		t.Run(name, func(t *testing.T) {
			a, _ := setup(t)
			a.Now = func() time.Time { return time.Now().UTC() }
			if paused {
				apply(t, a, "http://127.0.0.1:1")
				if _, err := a.Lifecycle(ctx, "api", LifecycleRequest{Action: "pause"}); err != nil {
					t.Fatal(err)
				}
			}
			run, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			var before, after syscall.Rusage
			if err := syscall.Getrusage(syscall.RUSAGE_SELF, &before); err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			if err := a.Run(run); err != nil && err != context.DeadlineExceeded {
				t.Fatal(err)
			}
			if err := syscall.Getrusage(syscall.RUSAGE_SELF, &after); err != nil {
				t.Fatal(err)
			}
			cpu := float64(after.Utime.Sec-before.Utime.Sec+after.Stime.Sec-before.Stime.Sec) +
				float64(after.Utime.Usec-before.Utime.Usec+after.Stime.Usec-before.Stime.Usec)/1e6
			t.Logf("wall %.2fs, CPU %.4fs, one-core CPU %.3f%%", time.Since(started).Seconds(), cpu, 100*cpu/time.Since(started).Seconds())
		})
	}
}
