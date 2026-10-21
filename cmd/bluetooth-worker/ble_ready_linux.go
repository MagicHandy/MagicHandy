package main

import (
	"context"
	"tinygo.org/x/bluetooth"
)

// BlueZ's connection operation already owns device discovery/readiness.
func prepareNativeConnection(ctx context.Context, _ *bluetooth.Adapter, _ string) error {
	return ctx.Err()
}
func awaitNativeConnection(ctx context.Context, _ bluetooth.Device) error { return ctx.Err() }
