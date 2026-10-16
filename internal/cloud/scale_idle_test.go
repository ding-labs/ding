package cloud

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This diagnostic has no load generator or SQL-backed metrics scrape. It counts
// real worker transactions, including the minute reconciliation, with retention
// transactions reported separately. It is not a throughput qualification.
func TestLocalIdleScale(t *testing.T) {
	backend := os.Getenv("DING_IDLE_BACKEND")
	if backend == "" {
		t.Skip("opt-in idle worker diagnostic")
	}
	out := os.Getenv("DING_IDLE_OUTPUT")
	if out == "" {
		t.Fatal("DING_IDLE_OUTPUT required")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{10, 100, 1000} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			c := scaleConfig{Name: "idle", Backend: backend, PoolLimit: 4, Workspaces: n, Rate: 1, Seconds: 75}
			if err := c.validate(); err != nil {
				t.Fatal(err)
			}
			lab := newScaleLab(t, c)
			snapshot := func() (string, map[string]float64) {
				var buffer bytes.Buffer
				lab.server.Pool.connectionMetrics(&buffer, lab.tenants)
				counts := map[string]float64{}
				for _, line := range strings.Split(buffer.String(), "\n") {
					key, value, ok := strings.Cut(line, " ")
					if ok {
						counts[key], _ = strconv.ParseFloat(value, 64)
					}
				}
				return buffer.String(), counts
			}
			time.Sleep(5 * time.Second) // Startup is outside the measurement window.
			before, a := snapshot()
			started := time.Now()
			time.Sleep(75 * time.Second) // Covers at least one reconciliation pass.
			after, b := snapshot()
			seconds := time.Since(started).Seconds()
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			key := func(operation string) string {
				return `ding_runtime_operation_seconds_count{backend="postgres",operation="` + operation + `"}`
			}
			runtimeRate := (b[key("sql_begin")] - a[key("sql_begin")]) / seconds
			maintenanceRate := (b[key("sql_maintenance_begin")] - a[key("sql_maintenance_begin")]) / seconds
			report := map[string]any{"backend": backend, "workspaces": n, "seconds": seconds, "startedAt": started.UTC(), "before": before, "after": after, "heapBytes": memory.HeapAlloc, "goroutines": runtime.NumGoroutine(), "postgresRuntimeTXPerSecond": runtimeRate, "postgresMaintenanceTXPerSecond": maintenanceRate, "postgresIdleBudgetPass": backend == "postgres" && runtimeRate <= 5}
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, backend+"-"+strconv.Itoa(n)+".json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			for _, tenant := range lab.tenants {
				if tenant.failed.Load() {
					t.Fatal("worker stopped during idle diagnostic")
				}
			}
			t.Logf("%s workspaces=%d runtime TX/s=%.2f maintenance TX/s=%.2f heap=%d", backend, n, runtimeRate, maintenanceRate, memory.HeapAlloc)
		})
	}
}
