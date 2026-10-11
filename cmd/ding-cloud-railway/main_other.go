//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "ding-cloud-railway is a Linux container entrypoint; use ding-cloud on other hosts")
	os.Exit(1)
}
