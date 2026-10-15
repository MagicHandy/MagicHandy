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
	// ErrDiscoveryFailed is a transient failure before any device command was
	// admitted; it is distinct from a device that lacks the required service.
	ErrDiscoveryFailed      = errors.New("bluetooth service discovery did not complete; retry the connection")
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
	// UnacknowledgedWrites means GATT accepts only Write Without Response.
	// Native completion then confirms queuing, never delivery to the device.
	UnacknowledgedWrites bool
}

// ConnectRequest supplies private reconnection identity and bounded deadlines.
type ConnectRequest struct {
	RememberedAddress string
	// SelectedAddress forbids falling back to another device if this address fails.
	SelectedAddress bool
	ConnectTimeout  time.Duration
	ScanTimeout     time.Duration
}

// Candidate is an unverified advertisement. Address stays on the host and must
// not appear in logs or diagnostics. Signal strength is not device identity.
type Candidate struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	RSSI    int    `json:"rssi"`
}

// Discoverer scans without connecting to or writing any peripheral.
type Discoverer interface {
	Discover(context.Context, time.Duration) ([]Candidate, error)
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
