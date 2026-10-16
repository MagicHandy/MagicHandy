package estim

import "github.com/mapledaemon/MagicHandy/internal/ble"

// LinkEventKind preserves the Coyote connector event contract.
type LinkEventKind = ble.LinkEventKind

// LinkEvent is a byte-link event interpreted by the Coyote driver.
type LinkEvent = ble.LinkEvent

// DeviceInfo contains the private connection identity.
type DeviceInfo = ble.DeviceInfo

// ConnectRequest selects a remembered device and connection deadlines.
type ConnectRequest = ble.ConnectRequest

// Link is the Coyote driver's shared native byte channel.
type Link = ble.Link

// Connector supplies a Coyote link, including simulated implementations.
type Connector = ble.Connector

// Link event kinds retain source compatibility for the Coyote driver.
const (
	LinkNotification = ble.LinkNotification
	LinkBattery      = ble.LinkBattery
	LinkLost         = ble.LinkLost
	LinkWriteFailed  = ble.LinkWriteFailed
)

// Connection errors are shared with the native Bluetooth client.
var (
	ErrNotFound             = ble.ErrNotFound
	ErrBluetoothUnavailable = ble.ErrBluetoothUnavailable
	ErrUnsupportedDevice    = ble.ErrUnsupportedDevice
	ErrHelperMissing        = ble.ErrHelperMissing
	ErrLinkClosed           = ble.ErrLinkClosed
)
