package transport

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/handyble"
)

// NativeBluetoothName identifies native Handy HSP diagnostics.
const NativeBluetoothName = "native_bluetooth_hsp"

// NativeBluetoothStatus is backend-authoritative and contains no hardware address.
type NativeBluetoothStatus struct {
	Connected     bool   `json:"connected"`
	Connecting    bool   `json:"connecting"`
	Simulated     bool   `json:"simulated"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	DeviceName    string `json:"device_name,omitempty"`
	ClockSynced   bool   `json:"clock_synced"`
	WriteBytes    int    `json:"write_bytes"`
	PlaybackState string `json:"playback_state"`
}

// NativeBluetoothOwner owns the host's Handy connection, independently of any
// browser tab or playback run. Each transport captures its connection generation.
type NativeBluetoothOwner struct {
	connector      ble.Connector
	mu             sync.Mutex
	session        *handyble.Session
	generation     uint64
	connectingDone chan struct{}
	cancel         context.CancelFunc
	device         ble.DeviceInfo
	lastError      string
}

// NewNativeBluetoothOwner creates a disconnected host-owned Handy connection.
func NewNativeBluetoothOwner(connector ble.Connector) *NativeBluetoothOwner {
	return &NativeBluetoothOwner{connector: connector}
}

// SessionID identifies the current connection generation.
func (o *NativeBluetoothOwner) SessionID() uint64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.generation
}

// Status returns backend connection state without hardware identifiers.
func (o *NativeBluetoothOwner) Status() NativeBluetoothStatus {
	o.mu.Lock()
	defer o.mu.Unlock()
	status := NativeBluetoothStatus{Connecting: o.connectingDone != nil, Simulated: o.connector.Simulated(), Status: "disconnected", Message: o.lastError, PlaybackState: "unknown"}
	if o.connectingDone != nil {
		status.Status = "connecting"
	}
	if o.session != nil {
		s := o.session.Snapshot()
		status.Connected, status.ClockSynced, status.PlaybackState = s.Ready, s.ClockSynced, s.PlaybackState
		status.DeviceName, status.WriteBytes = o.device.Name, o.device.MaxWriteBytes
		if s.Ready {
			status.Status = "connected"
		} else {
			status.Status, status.Message = "disconnected", "The native Handy Bluetooth connection ended. Reconnect to continue."
		}
	}
	return status
}

// Connect returns private device information only to the host-owned settings
// layer for remembering the selected address. It never starts playback.
func (o *NativeBluetoothOwner) Connect(ctx context.Context, request ble.ConnectRequest) (ble.DeviceInfo, error) {
	o.mu.Lock()
	if o.connectingDone != nil {
		o.mu.Unlock()
		return ble.DeviceInfo{}, errors.New("native Bluetooth is already connecting")
	}
	if o.session != nil && o.session.Snapshot().Ready {
		device := o.device
		o.mu.Unlock()
		return device, nil
	}
	old := o.session
	o.session = nil
	o.generation++
	generation := o.generation
	ctx, cancel := context.WithCancel(ctx)
	o.cancel, o.lastError = cancel, ""
	done := make(chan struct{})
	o.connectingDone = done
	o.mu.Unlock()
	defer cancel()
	defer func() {
		o.mu.Lock()
		if o.connectingDone == done {
			o.connectingDone = nil
		}
		o.mu.Unlock()
		close(done)
	}()
	if old != nil {
		_ = old.Close()
	}
	link, err := o.connector.Connect(ctx, request)
	var session *handyble.Session
	if err == nil {
		session, err = handyble.NewSession(link)
		if err == nil {
			err = session.Initialize(ctx)
		}
	}
	o.mu.Lock()
	if err == nil && (generation != o.generation || ctx.Err() != nil) {
		err = errors.New("native Bluetooth connection canceled")
	}
	if generation == o.generation {
		o.cancel = nil
		if err == nil {
			o.session, o.device = session, link.Device()
		} else {
			o.lastError = "Native Bluetooth could not connect. Check pairing mode, firmware 4, and the host Bluetooth adapter."
		}
	}
	o.mu.Unlock()
	if err != nil {
		if session != nil {
			_ = session.Close()
		} else if link != nil {
			_ = link.Close()
		}
		return ble.DeviceInfo{}, err
	}
	return link.Device(), nil
}

// Interrupt also cancels connection setup, so a global Stop cannot be followed
// by an old pending connection becoming ready.
func (o *NativeBluetoothOwner) Interrupt() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
		o.generation++
	}
	if o.session != nil {
		o.session.Interrupt()
	}
}

// Disconnect cancels setup or stops and closes an established connection.
func (o *NativeBluetoothOwner) Disconnect(ctx context.Context) error {
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
	session := o.session
	connectingDone := o.connectingDone
	o.session, o.lastError = nil, ""
	o.generation++
	o.mu.Unlock()
	var waitErr error
	if connectingDone != nil {
		select {
		case <-connectingDone:
		case <-ctx.Done():
			waitErr = ctx.Err()
		}
	}
	if session == nil {
		return waitErr
	}
	session.Interrupt()
	_, err := session.Execute(ctx, handyble.Request{Method: handyble.StopCurrentMode})
	return errors.Join(waitErr, err, session.Close())
}

// Close performs bounded shutdown of this connection owner.
func (o *NativeBluetoothOwner) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return o.Disconnect(ctx)
}

// NewTransport binds shared HSP mapping to the current native connection.
func (o *NativeBluetoothOwner) NewTransport(options HandyBluetoothOptions) *HandyBluetoothTransport {
	generation := o.SessionID()
	transport := newHandyBluetoothTransport(nativeHandyChannel{NativeBluetoothOwner: o, generation: generation}, options, NativeBluetoothName)
	transport.sessionID = generation
	return transport
}

type nativeHandyChannel struct {
	*NativeBluetoothOwner
	generation uint64
}

func (c nativeHandyChannel) Interrupt() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation == c.NativeBluetoothOwner.generation && c.session != nil {
		c.session.Interrupt()
	}
}

func (c nativeHandyChannel) Snapshot() handyBluetoothChannelStatus {
	s := c.Status()
	if c.generation != c.SessionID() {
		return handyBluetoothChannelStatus{Status: "connection_retired", Message: "This Bluetooth connection was replaced."}
	}
	return handyBluetoothChannelStatus{Ready: s.Connected, Status: s.Status, Message: s.Message, PlaybackState: s.PlaybackState}
}

func (c nativeHandyChannel) SendCommand(ctx context.Context, _ CommandKind, path string, body map[string]any) HandyBluetoothAck {
	ack := HandyBluetoothAck{Transport: NativeBluetoothName, Status: "not_connected"}
	c.mu.Lock()
	session, generation := c.session, c.NativeBluetoothOwner.generation
	c.mu.Unlock()
	expected, _ := ctx.Value(bluetoothSessionKey{}).(uint64)
	if session == nil || c.generation != generation || (expected != 0 && expected != generation) {
		ack.Error = "Native Bluetooth is not connected to this playback session."
		return ack
	}
	request, err := nativeHandyRequest(path, body)
	if err != nil {
		ack.Status, ack.Error = "invalid_command", err.Error()
		return ack
	}
	response, err := session.Execute(ctx, request)
	ack.ID = strconv.FormatUint(uint64(response.ID), 10)
	if err != nil {
		ack.Status, ack.Error = "device_error", err.Error()
		return ack
	}
	ack.OK, ack.Status = true, "native_write_completed"
	if response.State != nil {
		ack.Response = map[string]any{"hsp_state": map[string]any{"play_state": response.State.PlayState}}
	}
	return ack
}
