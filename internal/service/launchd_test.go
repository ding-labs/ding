package service

import (
	"context"
	"testing"
)

type exitCode int

func (e exitCode) Error() string { return "test process exit" }
func (e exitCode) ExitCode() int { return int(e) }

func TestLaunchdRetriesTransientTeardownOnly(t *testing.T) {
	for _, code := range []int{5, 1} {
		calls := 0
		m := Manager{UID: "501", Definition: Definition{Path: "owned.plist"}, Run: func(context.Context, string, ...string) ([]byte, error) {
			calls++
			if calls == 1 {
				return nil, exitCode(code)
			}
			return nil, nil
		}}
		err := m.bootstrap(context.Background())
		if code == 5 && (err != nil || calls != 2) {
			t.Fatal("transient teardown did not recover", calls, err)
		}
		if code == 1 && (err == nil || calls != 1) {
			t.Fatal("retried nontransient failure", calls, err)
		}
	}
}
