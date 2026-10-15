package cloud

import (
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

// Profiles cover only the measured load interval, excluding enrollment,
// database seeding and the exhaustive post-run receipt reconciliation.
func scaleProfiles(t *testing.T, out string) func() {
	t.Helper()
	f, err := mcpconfig.CreatePrivate(filepath.Join(out, "cpu.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	previous := runtime.SetMutexProfileFraction(10)
	runtime.SetBlockProfileRate(1000000)
	return func() {
		pprof.StopCPUProfile()
		f.Close()
		runtime.SetMutexProfileFraction(previous)
		runtime.SetBlockProfileRate(0)
		for _, name := range []string{"mutex", "block", "heap"} {
			p, err := mcpconfig.CreatePrivate(filepath.Join(out, name+".pprof"))
			if err != nil {
				t.Fatal(err)
			}
			err = pprof.Lookup(name).WriteTo(p, 0)
			p.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
