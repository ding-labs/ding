package watchcli

import (
	"fmt"
	"testing"
)

func TestApplicationExitStatus(t *testing.T) {
	for _, n := range []int{1, 7, 130, 143, 255} {
		if got := ExitCode(fmt.Errorf("wrapped: %w", &ExitStatus{Code: n})); got != n {
			t.Fatalf("got %d, want %d", got, n)
		}
	}
	if ExitCode(nil) != 0 || ExitCode(fmt.Errorf("failed")) != 1 {
		t.Fatal("ordinary exit status changed")
	}
}
