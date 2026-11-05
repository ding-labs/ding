package main

import (
	"context"
	"testing"
)

func TestFixtureRejectsNonLoopbackListeners(t *testing.T) {
	for _, address := range []string{":8787", "0.0.0.0:8787", "[::]:8787", "example.com:8787"} {
		if err := run(context.Background(), address); err == nil {
			t.Fatalf("accepted %s", address)
		}
	}
}

func TestFixtureStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
}
