// Package ble defines the byte-oriented contract between a device driver and
// an isolated native Bluetooth worker. It contains no motion or radio code.
package ble

import (
	"context"
	"errors"
	"time"
)

// Actionable connection failures. Device-specific UI supplies pairing guidance.
var (
	ErrNotFound             = errors.New("no matching Bluetooth device was found")
	ErrBluetoothUnavailable = errors.New("bluetooth is off or this PC has no Bluetooth adapter")
	ErrUnsupportedDevice    = errors.New("the device does not offer the required Bluetooth service")
	ErrHelperMissing        = errors.New("the native Bluetooth helper is missing or incompatible; update this installation")
	ErrLinkClosed           = errors.New("the Bluetooth connection is closed")
)

// LinkEventKind identifies a native connection event.
type LinkEventKind string

// Link event kinds contain no device protocol semantics.
const (
	LinkNotification LinkEventKind = "notification"
	LinkBattery      LinkEventKind = "battery"
	LinkLost         LinkEventKind = "lost"
	LinkWriteFailed  LinkEventKind = "write_failed"
)

// LinkEvent carries raw device bytes or a connection lifecycle change.
type LinkEvent struct {
	Kind    LinkEventKind
	Payload []byte
	Battery int
	Err     error
}

// DeviceInfo contains a private hardware identifier; never export Address in
// diagnostics or expose it to clients without host configuration permission.
type DeviceInfo struct {
	Name          string
	Address       string
	MaxWriteBytes int
}

// ConnectRequest supplies private reconnection identity and bounded deadlines.
type ConnectRequest struct {
	RememberedAddress string
	ConnectTimeout    time.Duration
	ScanTimeout       time.Duration
}

// Link owns one connection. Write waits for native API completion, not motor
// feedback or a firmware response. A canceled uncertain write retires the link.
type Link interface {
	Write(context.Context, []byte, bool) error
	ReadBattery(context.Context) (int, error)
	// SetLastWill is best effort on stdin EOF; process termination may prevent it.
	SetLastWill([]byte) error
	Events() <-chan LinkEvent
	Device() DeviceInfo
	Close() error
}

// Connector creates one independently owned byte link.
type Connector interface {
	Connect(context.Context, ConnectRequest) (Link, error)
	Simulated() bool
}
