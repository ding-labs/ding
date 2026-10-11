package watchcli

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestCancelledLogInputUnblocksSilentPipe(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	stop := interruptLogInput(ctx, r)
	defer stop()
	done := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, r); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected interrupted input")
		}
	case <-time.After(time.Second):
		t.Fatal("silent pipe remained blocked")
	}
}
