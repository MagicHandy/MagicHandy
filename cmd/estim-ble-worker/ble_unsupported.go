//go:build !windows && !linux

package main

import (
	"context"
	"fmt"
	"runtime"

	"github.com/mapledaemon/MagicHandy/internal/estim/bleworker"
)

// unsupportedAdapter reports that this platform has no CGO-free radio stack.
type unsupportedAdapter struct{}

func newAdapter() bleworker.Adapter {
	return unsupportedAdapter{}
}

func (unsupportedAdapter) Enable() error {
	return fmt.Errorf("%w: Coyote Bluetooth is not supported on %s", bleworker.ErrUnavailable, runtime.GOOS)
}

func (unsupportedAdapter) ConnectAddress(context.Context, string) (bleworker.Peripheral, error) {
	return nil, bleworker.ErrUnavailable
}

func (unsupportedAdapter) ScanName(context.Context, string) (string, error) {
	return "", bleworker.ErrUnavailable
}
