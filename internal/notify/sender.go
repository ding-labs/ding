//go:build !linux

package notify

import "context"

func NewSender(_ context.Context, stateDir string) (func(context.Context, Message) error, func()) {
	return func(ctx context.Context, message Message) error {
		message.StateDir = stateDir
		return Send(ctx, message)
	}, func() {}
}
