package estim

import (
	"context"
	"errors"
	"time"
)

// Connection failures a user can act on. Messages are user-facing; the UI
// localizes them by their exact text.
var (
	ErrNotFound             = errors.New("no Coyote was found; turn it on, put it in pairing mode by pulling both switches down, and try again")
	ErrBluetoothUnavailable = errors.New("bluetooth is off or this PC has no Bluetooth adapter")
	ErrUnsupportedDevice    = errors.New("the device does not offer the Coyote 3.0 pulse service")
	ErrHelperMissing        = errors.New("the Coyote Bluetooth helper is missing from this installation")
	ErrLinkClosed           = errors.New("the Coyote connection is closed")
)

// LinkEventKind classifies asynchronous link events.
type LinkEventKind string

// Link event kinds.
const (
	// LinkNotification carries a raw 0x150B notification.
	LinkNotification LinkEventKind = "notification"
	// LinkBattery carries a battery percentage.
	LinkBattery LinkEventKind = "battery"
	// LinkLost reports that the device connection ended unexpectedly.
	LinkLost LinkEventKind = "lost"
	// LinkWriteFailed reports that an unacknowledged stream write failed.
	LinkWriteFailed LinkEventKind = "write_failed"
)

// LinkEvent is one asynchronous event from a connected device.
type LinkEvent struct {
	Kind    LinkEventKind
	Payload []byte
	Battery int
	Err     error
}

// DeviceInfo identifies the connected device. The address is a hardware
// identifier kept for reconnection; it is not shown to non-host clients.
type DeviceInfo struct {
	Name    string
	Address string
}

// ConnectRequest prefers a remembered address, which recent Coyote firmware
// accepts without pairing mode, and falls back to a name scan that needs it.
type ConnectRequest struct {
	RememberedAddress string
	ConnectTimeout    time.Duration
	ScanTimeout       time.Duration
}

// Link is one connected Coyote. Writes go to the command characteristic in
// call order. An acknowledged write returns after the radio accepted it.
type Link interface {
	Write(ctx context.Context, payload []byte, acknowledged bool) error
	ReadBattery(ctx context.Context) (int, error)
	// SetLastWill stores the packet a worker writes if the core disappears
	// while connected. Losing the core also ends output on its own, because
	// the device plays nothing after its last 100 ms packet.
	SetLastWill(payload []byte) error
	Events() <-chan LinkEvent
	Device() DeviceInfo
	Close() error
}

// Connector opens links. Implementations: the Bluetooth worker process and the
// in-process simulator.
type Connector interface {
	Connect(ctx context.Context, request ConnectRequest) (Link, error)
	// Simulated reports whether links reach only the in-process simulator.
	Simulated() bool
}
