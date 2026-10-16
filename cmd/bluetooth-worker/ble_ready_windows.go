package main

import (
	"context"
	"sync/atomic"
	"time"

	"tinygo.org/x/bluetooth"

	"github.com/mapledaemon/MagicHandy/internal/ble/bleworker"
)

// WinRT address lookup depends on its advertisement cache. Refresh only the
// selected address in this connection worker; never choose another peripheral.
// This also handles bound COYOTEs whose advertisements omit the device name.
type connectionScanner interface {
	Scan(func(*bluetooth.Adapter, bluetooth.ScanResult)) error
	StopScan() error
}

func prepareNativeConnection(ctx context.Context, adapter connectionScanner, address string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var seen atomic.Bool
	finished := make(chan error, 1)
	go func() {
		finished <- adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
			if normalizeAddress(result.Address.String()) == normalizeAddress(address) {
				seen.Store(true)
				_ = adapter.StopScan()
			}
		})
	}()
	select {
	case err := <-finished:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		_ = adapter.StopScan()
		<-finished
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !seen.Load() {
		return bleworker.ErrNotFound
	}
	return nil
}

// Creating a BluetoothLEDevice is not an established connection. GATT must
// wait for the connection state, bounded by the original connect deadline.
func awaitNativeConnection(ctx context.Context, device bluetooth.Device) error {
	return awaitConnected(ctx, device.Connected)
}

func awaitConnected(ctx context.Context, connected func() (bool, error)) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ready, err := connected()
		if err != nil || ready {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
