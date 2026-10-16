//go:build !windows && !linux

package main

import (
	"context"
	"fmt"
	"github.com/mapledaemon/MagicHandy/internal/ble"
	"runtime"

	"github.com/mapledaemon/MagicHandy/internal/ble/bleworker"
)

// unsupportedAdapter reports that this platform has no CGO-free radio stack.
type unsupportedAdapter struct{}

func newAdapter() bleworker.Adapter {
	return unsupportedAdapter{}
}

func (unsupportedAdapter) Enable() error {
	return fmt.Errorf("%w: native Bluetooth is not supported on %s", bleworker.ErrUnavailable, runtime.GOOS)
}

func (unsupportedAdapter) ConnectAddress(context.Context, string, ble.Profile) (bleworker.Peripheral, error) {
	return nil, bleworker.ErrUnavailable
}

func (unsupportedAdapter) ScanName(context.Context, ble.Profile) (string, error) {
	return "", bleworker.ErrUnavailable
}
