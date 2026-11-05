// ding-health-fixture runs a controllable synthetic service on loopback.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/ding-labs/ding/internal/demo"
)

func main() {
	address := flag.String("listen", "127.0.0.1:8787", "loopback address for the synthetic service")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, *address); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("fixture requires a loopback IP address, such as 127.0.0.1:8787")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: demo.HealthFixture(), ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	fmt.Printf("SYNTHETIC service only: http://%s/health\nSet healthy, failing, or timeout with PUT /mode. Ctrl+C stops the fixture.\n", listener.Addr())
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		// Close also cancels deliberately stalled timeout requests.
		_ = server.Close()
		err = <-done
	case err = <-done:
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
