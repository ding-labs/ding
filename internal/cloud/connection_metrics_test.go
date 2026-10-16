package cloud

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"
)

func TestConnectionChurnIncludesEveryWorkspacePool(t *testing.T) {
	var total sql.DBStats
	addConnectionStats(&total, sql.DBStats{MaxIdleClosed: 2, MaxIdleTimeClosed: 3, MaxLifetimeClosed: 4})
	addConnectionStats(&total, sql.DBStats{MaxIdleClosed: 10, MaxIdleTimeClosed: 20, MaxLifetimeClosed: 30})
	var out bytes.Buffer
	writeConnectionStats(&out, "sqlite", "snapshot_readers", total)
	for _, want := range []string{
		`reason="idle_limit"} 12`, `reason="idle_time"} 23`, `reason="lifetime"} 34`,
	} {
		if !strings.Contains(out.String(), want+"\n") {
			t.Fatalf("missing connection churn: %s", want)
		}
	}
}
