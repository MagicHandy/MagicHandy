package transport

import "context"

// handyBluetoothChannel separates Handy HSP mapping from connection ownership.
// Browser and native channels have different lifetimes, but share stream indices,
// percent conversion, admission fencing and the engine-facing transport.
type handyBluetoothChannel interface {
	SessionID() uint64
	Snapshot() handyBluetoothChannelStatus
	SendCommand(context.Context, CommandKind, string, map[string]any) HandyBluetoothAck
}

type handyBluetoothChannelStatus struct {
	Ready                          bool
	Status, Message, PlaybackState string
	LastAck                        *HandyBluetoothAck
}

// HandyBluetoothAck describes command completion. OK acknowledges dispatch;
// firmware response data, if present, is reported separately in Response.
type HandyBluetoothAck struct {
	ID            string         `json:"id,omitempty"`
	OK            bool           `json:"ok"`
	Status        string         `json:"status,omitempty"`
	Transport     string         `json:"transport"`
	ElapsedMillis float64        `json:"elapsed_ms,omitempty"`
	Error         string         `json:"error,omitempty"`
	Response      map[string]any `json:"response,omitempty"`
}

type browserHandyChannel struct{ *BrowserBluetoothBridge }

func (c browserHandyChannel) Snapshot() handyBluetoothChannelStatus {
	s := c.BrowserBluetoothBridge.Snapshot()
	status := handyBluetoothChannelStatus{Ready: s.Ready, Status: s.Status, Message: s.Message, LastAck: s.LastAck}
	if s.HSPState != nil {
		status.PlaybackState = s.HSPState.PlayState
	}
	return status
}
