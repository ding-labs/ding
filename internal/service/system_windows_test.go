package service

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestSCMReportsReadinessAndDrainsOnShutdown(t *testing.T) {
	requests := make(chan svc.ChangeRequest, 1)
	status := make(chan svc.Status, 10)
	finished := make(chan uint32, 1)
	drained := make(chan struct{})
	h := &systemHandler{ready: func(context.Context) error { return nil }, run: func(ctx context.Context) error { <-ctx.Done(); close(drained); return nil }}
	go func() { _, code := h.Execute(nil, requests, status); finished <- code }()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case s := <-status:
			if s.State == svc.Running {
				if s.Accepts != svc.AcceptStop|svc.AcceptShutdown {
					t.Fatal("shutdown not advertised")
				}
				requests <- svc.ChangeRequest{Cmd: svc.Shutdown}
			}
		case code := <-finished:
			if code != 0 {
				t.Fatal("failed shutdown", code)
			}
			select {
			case <-drained:
			default:
				t.Fatal("SCM returned before daemon drained")
			}
			return
		case <-deadline:
			t.Fatal("SCM lifecycle timed out")
		}
	}
}
