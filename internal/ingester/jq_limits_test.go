package ingester

import (
	"context"
	"testing"
	"time"
)

func TestJQBudgets(t *testing.T) {
	for _, tc := range []struct {
		expr         string
		count, bytes int
	}{
		{"repeat(.)", 10, 1024}, {".", 10, 1}, {". + {padding: (\"a\" * 1000)}", 10, 100},
	} {
		code, err := CompileJQ(tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = RunJQContext(context.Background(), code, []byte(`{"metric":"m","value":1}`), tc.count, tc.bytes); err == nil {
			t.Fatal(tc)
		}
	}
	code, _ := CompileJQ("def forever: forever; forever")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := RunJQContext(ctx, code, []byte(`{}`), 100, 1024); err == nil {
		t.Fatal("infinite jq accepted")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation failed")
	}
}

func TestStrictTimestamps(t *testing.T) {
	for _, value := range []string{`null`, `true`, `"bad"`, `1e20`} {
		if _, err := ParseJSONLine([]byte(`{"metric":"m","value":1,"timestamp":` + value + `}`)); err == nil {
			t.Fatal(value)
		}
	}
	ev, err := ParseJSONLine([]byte(`{"metric":"m","value":1,"timestamp":"2026-01-01T00:00:00.123Z"}`))
	if err != nil || ev[0].At.Nanosecond() != 123000000 {
		t.Fatal(ev, err)
	}
}
