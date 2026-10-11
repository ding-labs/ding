package watchcli

import (
	"bytes"
	"testing"
)

func TestNamedConnectionCannotFallBackToPersonalOrHost(t *testing.T) {
	for _, args := range [][]string{{"--connection", "missing", "watch", "list"}, {"--connection", "team", "daemon"}, {"--connection", "../team", "watch", "list"}, {"--connection", "team", "secret", "list"}} {
		var out bytes.Buffer
		if err := Execute("test", append([]string{"--state-dir", t.TempDir()}, args...), &out, &out); err == nil {
			t.Fatalf("unexpected success: %v", args)
		}
	}
}
