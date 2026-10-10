//go:build !windows

package service

import (
	"context"
	"fmt"
)

func RunSystem(string, func(context.Context) error, func(context.Context) error) error {
	return fmt.Errorf("Windows SCM hosting is only available on Windows; use launchd or systemd for this OS")
}
