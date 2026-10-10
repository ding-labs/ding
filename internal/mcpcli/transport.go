package mcpcli

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpserver"
	"github.com/ding-labs/ding/internal/mcpui"
)

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func serveHTTP(ctx context.Context, version, path, host string, port int) error {
	if path == "" || port < 1 || port > 65535 {
		return mcpconfig.ErrConfig
	}
	cfg, err := mcpconfig.LoadHTTP(path)
	if err != nil {
		return err
	}
	handler, close, err := mcpserver.HTTPHandler(mcpserver.HTTPOptions{Config: cfg, Version: version, HTML: mcpui.WorkspaceHTML})
	if err != nil {
		return err
	}
	defer close()
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 45 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
