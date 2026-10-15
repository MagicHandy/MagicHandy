// Package workerproto defines the NDJSON protocol between the MagicHandy core
// and the native Bluetooth worker process. The worker is a thin radio pipe:
// it connects, writes the bytes the core composed and reports what the device
// sends back. Every protocol decision stays in the core.
package workerproto

import "github.com/mapledaemon/MagicHandy/internal/ble"

// Version is the protocol version both sides must agree on in hello.
const Version = 4

// MaxLineBytes bounds one protocol line in either direction.
const MaxLineBytes = 64 * 1024

// Message types. Requests flow core to worker; replies carry the request ID.
// Unsolicited events carry no ID.
const (
	TypeHello      = "hello"
	TypeDiscover   = "discover"
	TypeConnect    = "connect"
	TypeWrite      = "write"
	TypeBattery    = "battery"
	TypeLastWill   = "last_will"
	TypeDisconnect = "disconnect"
	TypeShutdown   = "shutdown"

	TypeConnected    = "connected"
	TypeDiscovered   = "discovered"
	TypeWritten      = "written"
	TypeNotify       = "notify"
	TypeDisconnected = "disconnected"
	TypeError        = "error"
)

// Error codes reported by the worker.
const (
	CodeNotFound     = "not_found"
	CodeUnavailable  = "bluetooth_unavailable"
	CodeUnsupported  = "unsupported_device"
	CodeNotConnected = "not_connected"
	CodeBusy         = "busy"
	CodeWriteFailed  = "write_failed"
	CodeConnect      = "connect_failed"
	CodeDiscovery    = "discovery_failed"
	CodeBadRequest   = "bad_request"
	CodeAmbiguous    = "ambiguous_device"
)

// Message is one protocol line. Data is hex-encoded bytes.
type Message struct {
	Type                 string          `json:"type"`
	ID                   string          `json:"id,omitempty"`
	Protocol             int             `json:"protocol,omitempty"`
	Worker               string          `json:"worker,omitempty"`
	Profile              string          `json:"profile,omitempty"`
	Profiles             []string        `json:"profiles,omitempty"`
	MaxWriteBytes        int             `json:"max_write_bytes,omitempty"`
	UnacknowledgedWrites bool            `json:"unacknowledged_writes,omitempty"`
	Platform             string          `json:"platform,omitempty"`
	Name                 string          `json:"name,omitempty"`
	Address              string          `json:"address,omitempty"`
	SelectedAddress      bool            `json:"selected_address,omitempty"`
	Candidates           []ble.Candidate `json:"candidates,omitempty"`
	ConnectTimeout       int             `json:"connect_timeout_ms,omitempty"`
	ScanTimeout          int             `json:"scan_timeout_ms,omitempty"`
	Data                 string          `json:"data,omitempty"`
	// Ack selects GATT Write With Response. Every write carrying an ID gets
	// a worker completion reply, including Write Without Response.
	Ack     bool   `json:"ack,omitempty"`
	Percent *int   `json:"percent,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
}
