// Package workerproto defines the NDJSON protocol between the MagicHandy core
// and the Coyote Bluetooth worker process. The worker is a thin radio pipe:
// it connects, writes the bytes the core composed and reports what the device
// sends back. Every protocol decision stays in the core.
package workerproto

// Version is the protocol version both sides must agree on in hello.
const Version = 2

// MaxLineBytes bounds one protocol line in either direction.
const MaxLineBytes = 64 * 1024

// Message types. Requests flow core to worker; replies carry the request ID.
// Unsolicited events carry no ID.
const (
	TypeHello      = "hello"
	TypeConnect    = "connect"
	TypeWrite      = "write"
	TypeBattery    = "battery"
	TypeLastWill   = "last_will"
	TypeDisconnect = "disconnect"
	TypeShutdown   = "shutdown"

	TypeConnected    = "connected"
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
	CodeBadRequest   = "bad_request"
)

// Message is one protocol line. Data is hex-encoded bytes.
type Message struct {
	Type           string `json:"type"`
	ID             string `json:"id,omitempty"`
	Protocol       int    `json:"protocol,omitempty"`
	Worker         string `json:"worker,omitempty"`
	Platform       string `json:"platform,omitempty"`
	Name           string `json:"name,omitempty"`
	Address        string `json:"address,omitempty"`
	ConnectTimeout int    `json:"connect_timeout_ms,omitempty"`
	ScanTimeout    int    `json:"scan_timeout_ms,omitempty"`
	Data           string `json:"data,omitempty"`
	// Ack selects GATT Write With Response. Every write carrying an ID gets
	// a worker completion reply, including Write Without Response.
	Ack     bool   `json:"ack,omitempty"`
	Percent *int   `json:"percent,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
}
