package runtimeperf

import "context"

type maintenanceKey struct{}

// WithMaintenance attributes background retention transactions separately from
// request/scheduling transactions. It has no effect on authorization or storage.
func WithMaintenance(ctx context.Context) context.Context {
	return context.WithValue(ctx, maintenanceKey{}, true)
}

func IsMaintenance(ctx context.Context) bool {
	value, _ := ctx.Value(maintenanceKey{}).(bool)
	return value
}
