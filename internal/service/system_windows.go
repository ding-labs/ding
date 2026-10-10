package service

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
)

// RunSystem is the Windows SCM entrypoint. It runs the same daemon in-process;
// stop/shutdown cancels that daemon and waits for its store/outbox to drain.
func RunSystem(name string, run func(context.Context) error, ready func(context.Context) error) error {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return fmt.Errorf("service host must be launched by Windows Service Control Manager")
	}
	return svc.Run(name, &systemHandler{run: run, ready: ready})
}

type systemHandler struct {
	run   func(context.Context) error
	ready func(context.Context) error
}

func (h *systemHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()
	readiness := make(chan error, 1)
	go func() {
		probe, stop := context.WithTimeout(ctx, 30*time.Second)
		defer stop()
		readiness <- h.ready(probe)
	}()
	current := svc.Status{State: svc.StartPending, WaitHint: 30000}
	status <- current
	drain := func() (bool, uint32) {
		status <- svc.Status{State: svc.StopPending, WaitHint: 45000}
		cancel()
		select {
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		case <-time.After(45 * time.Second):
			return true, 2
		}
	}
	for {
		select {
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		case err := <-readiness:
			readiness = nil
			if err != nil {
				return drain()
			}
			current = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
			status <- current
		case request, ok := <-requests:
			if !ok {
				return drain()
			}
			switch request.Cmd {
			case svc.Interrogate:
				status <- current
			case svc.Stop, svc.Shutdown:
				return drain()
			}
		}
	}
}
