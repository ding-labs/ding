package mcpcli

import (
	"context"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"io"
)

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// Replaced by authenticated HTTP wiring before the adapter is released.
func serveHTTP(context.Context, string, string, string, int) error { return mcpconfig.ErrConfig }
