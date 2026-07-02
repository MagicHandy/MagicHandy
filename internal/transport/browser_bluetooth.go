package transport

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// BrowserBluetoothName is the diagnostics transport name for browser-owned BLE.
	BrowserBluetoothName = "browser_bluetooth_hsp"

	defaultBluetoothCommandTimeout = 8 * time.Second
	defaultBluetoothStaleAfter     = 6 * time.Second
	defaultBluetoothQueueLimit     = 120
	defaultBluetoothBatchLimit     = 24
)

// BrowserBluetoothBridgeOption configures a browser-owned Bluetooth bridge.
type BrowserBluetoothBridgeOption func(*BrowserBluetoothBridge)

// BrowserBluetoothClientStatus is a browser tab status update.
type BrowserBluetoothClientStatus struct {
	ClientID   string `json:"client_id"`
	Connected  *bool  `json:"connected,omitempty"`
	Supported  *bool  `json:"supported,omitempty"`
	Status     string `json:"status,omitempty"`
	Message    string `json:"message,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	DeviceID   string `json:"device_id,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BrowserBluetoothBridgeCommand is the command shape consumed by the browser.
type BrowserBluetoothBridgeCommand struct {
	ID        string         `json:"id"`
	Path      string         `json:"path"`
	Kind      CommandKind    `json:"kind,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
	CreatedAt string         `json:"created_at"`
}

// BrowserBluetoothBridgeAck is the browser command result shape.
type BrowserBluetoothBridgeAck struct {
	ID            string         `json:"id,omitempty"`
	OK            bool           `json:"ok"`
	Status        string         `json:"status,omitempty"`
	Transport     string         `json:"transport"`
	ElapsedMillis float64        `json:"elapsed_ms,omitempty"`
	Error         string         `json:"error,omitempty"`
	Response      map[string]any `json:"response,omitempty"`
}

// BrowserBluetoothBridgeSnapshot is a safe bridge status view.
type BrowserBluetoothBridgeSnapshot struct {
	Transport         string                     `json:"transport"`
	Connected         bool                       `json:"connected"`
	Ready             bool                       `json:"ready"`
	Supported         bool                       `json:"supported"`
	Stale             bool                       `json:"stale"`
	Status            string                     `json:"status"`
	Message           string                     `json:"message"`
	DeviceName        string                     `json:"device_name,omitempty"`
	DeviceID          string                     `json:"device_id,omitempty"`
	Protocol          string                     `json:"protocol,omitempty"`
	ClientID          string                     `json:"client_id,omitempty"`
	Pending           int                        `json:"pending"`
	Inflight          int                        `json:"inflight"`
	LastSeenAgeMillis *float64                   `json:"last_seen_age_ms,omitempty"`
	LastError         string                     `json:"last_error,omitempty"`
	LastAck           *BrowserBluetoothBridgeAck `json:"last_ack,omitempty"`
}

// BrowserBluetoothError classifies bridge, browser, and device failures.
type BrowserBluetoothError struct {
	Status  string
	Message string
}

// Error returns the safe Bluetooth transport failure message.
func (e BrowserBluetoothError) Error() string {
	return e.Message
}

// BrowserBluetoothBridge is a synchronous server-side bridge for a browser-owned
// Web Bluetooth Handy link.
type BrowserBluetoothBridge struct {
	mu             sync.Mutex
	notify         chan struct{}
	clock          func() time.Time
	commandTimeout time.Duration
	staleAfter     time.Duration
	queueLimit     int
	batchLimit     int
	nextCommandID  int

	activeClientID string
	connected      bool
	supported      bool
	status         string
	message        string
	deviceName     string
	deviceID       string
	protocol       string
	lastSeenAt     time.Time
	lastError      string
	lastAck        *BrowserBluetoothBridgeAck
	pending        []BrowserBluetoothBridgeCommand
	inflight       map[string]BrowserBluetoothBridgeCommand
	acks           map[string]BrowserBluetoothBridgeAck
}

// NewBrowserBluetoothBridge returns a browser-owned Bluetooth bridge.
func NewBrowserBluetoothBridge(options ...BrowserBluetoothBridgeOption) *BrowserBluetoothBridge {
	bridge := &BrowserBluetoothBridge{
		notify:         make(chan struct{}),
		clock:          func() time.Time { return time.Now().UTC() },
		commandTimeout: defaultBluetoothCommandTimeout,
		staleAfter:     defaultBluetoothStaleAfter,
		queueLimit:     defaultBluetoothQueueLimit,
		batchLimit:     defaultBluetoothBatchLimit,
		nextCommandID:  1,
		supported:      true,
		status:         "disconnected",
		message:        "Bluetooth not connected.",
		inflight:       make(map[string]BrowserBluetoothBridgeCommand),
		acks:           make(map[string]BrowserBluetoothBridgeAck),
	}
	for _, option := range options {
		option(bridge)
	}
	return bridge
}

// WithBrowserBluetoothClock sets the bridge clock for tests.
func WithBrowserBluetoothClock(clock func() time.Time) BrowserBluetoothBridgeOption {
	return func(bridge *BrowserBluetoothBridge) {
		if clock != nil {
			bridge.clock = clock
		}
	}
}

// WithBrowserBluetoothCommandTimeout sets the bridge command timeout.
func WithBrowserBluetoothCommandTimeout(timeout time.Duration) BrowserBluetoothBridgeOption {
	return func(bridge *BrowserBluetoothBridge) {
		if timeout > 0 {
			bridge.commandTimeout = timeout
		}
	}
}

// WithBrowserBluetoothStaleAfter sets the bridge stale-tab window.
func WithBrowserBluetoothStaleAfter(timeout time.Duration) BrowserBluetoothBridgeOption {
	return func(bridge *BrowserBluetoothBridge) {
		if timeout > 0 {
			bridge.staleAfter = timeout
		}
	}
}

// ConnectClient marks a browser tab as the active Bluetooth owner.
func (b *BrowserBluetoothBridge) ConnectClient(status BrowserBluetoothClientStatus) BrowserBluetoothBridgeSnapshot {
	clientID := strings.TrimSpace(status.ClientID)
	if clientID == "" {
		return b.Snapshot()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.activeClientID != "" && clientID != b.activeClientID {
		b.failAllLocked("bridge_client_changed", "Bluetooth browser client changed.")
	} else if len(b.pending) > 0 || len(b.inflight) > 0 {
		b.failAllLocked("bridge_reconnected", "Bluetooth browser client reconnected.")
	}
	b.activeClientID = clientID
	b.connected = true
	b.supported = true
	if status.Supported != nil {
		b.supported = *status.Supported
	}
	b.status = "connected"
	if strings.TrimSpace(status.Status) != "" {
		b.status = safeShortString(status.Status, 40)
	}
	b.message = safeShortString(status.Message, 180)
	if b.message == "" {
		b.message = "Handy Bluetooth connected."
	}
	b.deviceName = safeShortString(status.DeviceName, 80)
	b.deviceID = safeShortString(status.DeviceID, 80)
	b.protocol = safeShortString(status.Protocol, 80)
	b.lastSeenAt = b.clock()
	b.lastError = ""
	b.broadcastLocked()
	return b.snapshotLocked()
}

// UpdateClient records a browser status or heartbeat.
func (b *BrowserBluetoothBridge) UpdateClient(status BrowserBluetoothClientStatus) BrowserBluetoothBridgeSnapshot {
	clientID := strings.TrimSpace(status.ClientID)

	b.mu.Lock()
	defer b.mu.Unlock()

	if clientID != "" {
		if b.activeClientID != "" && clientID != b.activeClientID && b.connected {
			return b.snapshotLocked()
		}
		b.activeClientID = clientID
	}
	if status.Supported != nil {
		b.supported = *status.Supported
	}
	if status.Connected != nil {
		b.connected = *status.Connected
	}
	if strings.TrimSpace(status.DeviceName) != "" {
		b.deviceName = safeShortString(status.DeviceName, 80)
	}
	if strings.TrimSpace(status.DeviceID) != "" {
		b.deviceID = safeShortString(status.DeviceID, 80)
	}
	if strings.TrimSpace(status.Protocol) != "" {
		b.protocol = safeShortString(status.Protocol, 80)
	}
	if strings.TrimSpace(status.Status) != "" {
		b.status = safeShortString(status.Status, 40)
	} else if b.connected {
		b.status = "connected"
	} else if !b.supported {
		b.status = "unsupported"
	} else {
		b.status = "disconnected"
	}
	if strings.TrimSpace(status.Message) != "" {
		b.message = safeShortString(status.Message, 180)
	} else if b.connected {
		b.message = "Handy Bluetooth connected."
	} else if !b.supported {
		b.message = "Web Bluetooth is not available in this browser."
	}
	if strings.TrimSpace(status.Error) != "" {
		b.lastError = safeShortString(status.Error, 180)
		b.message = b.lastError
	}
	b.lastSeenAt = b.clock()
	if !b.connected {
		b.failAllLocked("bridge_disconnected", b.messageOrDefaultLocked())
	}
	b.broadcastLocked()
	return b.snapshotLocked()
}

// DisconnectClient marks the active browser owner as disconnected.
func (b *BrowserBluetoothBridge) DisconnectClient(clientID string, message string) BrowserBluetoothBridgeSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()

	clientID = strings.TrimSpace(clientID)
	if clientID != "" && b.activeClientID != "" && clientID != b.activeClientID {
		return b.snapshotLocked()
	}
	b.connected = false
	b.status = "disconnected"
	b.message = safeShortString(message, 180)
	if b.message == "" {
		b.message = "Bluetooth disconnected."
	}
	b.lastSeenAt = b.clock()
	b.failAllLocked("bridge_disconnected", b.message)
	b.broadcastLocked()
	return b.snapshotLocked()
}

// SendCommand queues a command for the active browser and waits for its result.
func (b *BrowserBluetoothBridge) SendCommand(ctx context.Context, kind CommandKind, path string, body map[string]any) BrowserBluetoothBridgeAck {
	if ctx == nil {
		ctx = context.Background()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return b.failureAck("", "bridge_command_invalid", "missing Bluetooth command path", 0)
	}

	start := b.clock()
	deadline := time.Now().Add(b.commandTimeout)

	b.mu.Lock()
	defer b.mu.Unlock()

	if ack, ok := b.readinessFailureLocked("", start); ok {
		return ack
	}

	commandID := fmt.Sprintf("bt-%06d", b.nextCommandID)
	b.nextCommandID++
	command := BrowserBluetoothBridgeCommand{
		ID:        commandID,
		Path:      path,
		Kind:      kind,
		Body:      cloneBridgeBody(body),
		CreatedAt: start.Format(time.RFC3339Nano),
	}
	if len(b.pending) >= b.queueLimit {
		dropped := b.pending[0]
		b.pending = b.pending[1:]
		b.acks[dropped.ID] = b.failureAck(dropped.ID, "bridge_queue_overflow", "Bluetooth command queue overflow; dropped pending command.", 0)
	}
	b.pending = append(b.pending, command)
	b.broadcastLocked()

	for {
		if ack, ok := b.acks[commandID]; ok {
			delete(b.acks, commandID)
			ack.ID = commandID
			ack.Transport = BrowserBluetoothName
			b.lastAck = cloneAckPtr(ack)
			if !ack.OK {
				b.lastError = ack.Error
			} else {
				b.lastError = ""
			}
			return ack
		}
		if ack, ok := b.readinessFailureLocked(commandID, start); ok {
			b.removeCommandLocked(commandID)
			b.lastAck = cloneAckPtr(ack)
			b.lastError = ack.Error
			b.broadcastLocked()
			return ack
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			b.removeCommandLocked(commandID)
			ack := b.failureAck(commandID, "bridge_timeout", "Timed out waiting for browser Bluetooth command acknowledgement.", b.clock().Sub(start))
			b.lastAck = cloneAckPtr(ack)
			b.lastError = ack.Error
			b.broadcastLocked()
			return ack
		}

		notify := b.notify
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			b.mu.Lock()
			b.removeCommandLocked(commandID)
			ack := b.failureAck(commandID, "bridge_canceled", ctx.Err().Error(), b.clock().Sub(start))
			b.lastAck = cloneAckPtr(ack)
			b.lastError = ack.Error
			b.broadcastLocked()
			return ack
		case <-notify:
			b.mu.Lock()
		case <-time.After(minDuration(remaining, 250*time.Millisecond)):
			b.mu.Lock()
		}
	}
}

// NextCommands returns pending commands for a browser long poll.
func (b *BrowserBluetoothBridge) NextCommands(ctx context.Context, clientID string, wait time.Duration) ([]BrowserBluetoothBridgeCommand, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, errors.New("missing browser Bluetooth client id")
	}
	if wait < 0 {
		wait = 0
	}
	if wait > 10*time.Second {
		wait = 10 * time.Second
	}
	deadline := time.Now().Add(wait)

	b.mu.Lock()
	defer b.mu.Unlock()

	for len(b.pending) == 0 {
		if !b.isCurrentClientLocked(clientID) || !b.connected {
			return nil, nil
		}
		b.lastSeenAt = b.clock()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil
		}
		notify := b.notify
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			b.mu.Lock()
			return nil, ctx.Err()
		case <-notify:
			b.mu.Lock()
		case <-time.After(minDuration(remaining, 250*time.Millisecond)):
			b.mu.Lock()
		}
	}

	if !b.isCurrentClientLocked(clientID) || !b.connected {
		return nil, nil
	}
	limit := b.batchLimit
	if limit <= 0 {
		limit = defaultBluetoothBatchLimit
	}
	if limit > len(b.pending) {
		limit = len(b.pending)
	}
	commands := make([]BrowserBluetoothBridgeCommand, limit)
	copy(commands, b.pending[:limit])
	b.pending = b.pending[limit:]
	for _, command := range commands {
		b.inflight[command.ID] = cloneBridgeCommand(command)
	}
	b.lastSeenAt = b.clock()
	b.broadcastLocked()
	return cloneBridgeCommands(commands), nil
}

// Acknowledge records a browser command result.
func (b *BrowserBluetoothBridge) Acknowledge(clientID string, ack BrowserBluetoothBridgeAck) BrowserBluetoothBridgeSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isCurrentClientLocked(clientID) {
		return b.snapshotLocked()
	}
	ack.ID = strings.TrimSpace(ack.ID)
	if ack.ID == "" {
		return b.snapshotLocked()
	}
	delete(b.inflight, ack.ID)
	ack.Transport = BrowserBluetoothName
	if ack.Status == "" {
		if ack.OK {
			ack.Status = "browser_ack"
		} else {
			ack.Status = "device_error"
		}
	}
	if !ack.OK && strings.TrimSpace(ack.Error) != "" {
		ack.Error = safeShortString(ack.Error, 240)
		b.lastError = ack.Error
	}
	ack.Response = cloneBridgeBody(ack.Response)
	b.acks[ack.ID] = ack
	b.lastAck = cloneAckPtr(ack)
	b.lastSeenAt = b.clock()
	b.broadcastLocked()
	return b.snapshotLocked()
}

// Snapshot returns the current safe bridge status.
func (b *BrowserBluetoothBridge) Snapshot() BrowserBluetoothBridgeSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.snapshotLocked()
}

func (b *BrowserBluetoothBridge) readinessFailureLocked(commandID string, start time.Time) (BrowserBluetoothBridgeAck, bool) {
	if !b.supported {
		return b.failureAck(commandID, "browser_unsupported", "Web Bluetooth is not available in this browser.", b.clock().Sub(start)), true
	}
	if b.isStaleLocked() {
		return b.failureAck(commandID, "bridge_stale", "Bluetooth browser bridge is stale.", b.clock().Sub(start)), true
	}
	if !b.connected || b.activeClientID == "" {
		return b.failureAck(commandID, "bridge_unavailable", b.messageOrDefaultLocked(), b.clock().Sub(start)), true
	}
	return BrowserBluetoothBridgeAck{}, false
}

func (b *BrowserBluetoothBridge) failureAck(commandID string, status string, message string, elapsed time.Duration) BrowserBluetoothBridgeAck {
	if strings.TrimSpace(message) == "" {
		message = "Bluetooth command failed."
	}
	return BrowserBluetoothBridgeAck{
		ID:            commandID,
		OK:            false,
		Status:        status,
		Transport:     BrowserBluetoothName,
		ElapsedMillis: float64(elapsed.Milliseconds()),
		Error:         message,
	}
}

func (b *BrowserBluetoothBridge) isCurrentClientLocked(clientID string) bool {
	return clientID != "" && b.activeClientID == clientID
}

func (b *BrowserBluetoothBridge) isStaleLocked() bool {
	return b.connected &&
		b.activeClientID != "" &&
		!b.lastSeenAt.IsZero() &&
		b.clock().Sub(b.lastSeenAt) > b.staleAfter
}

func (b *BrowserBluetoothBridge) isReadyLocked() bool {
	return b.connected && b.supported && b.activeClientID != "" && !b.isStaleLocked()
}

func (b *BrowserBluetoothBridge) snapshotLocked() BrowserBluetoothBridgeSnapshot {
	stale := b.isStaleLocked()
	connected := b.connected && !stale
	status := b.status
	message := b.message
	if stale {
		status = "stale"
		message = "Bluetooth browser bridge is stale."
	}
	if status == "" {
		status = "disconnected"
	}
	if message == "" {
		message = "Bluetooth not connected."
	}
	var lastSeenAge *float64
	if !b.lastSeenAt.IsZero() {
		age := float64(b.clock().Sub(b.lastSeenAt).Milliseconds())
		if age < 0 {
			age = 0
		}
		lastSeenAge = &age
	}
	return BrowserBluetoothBridgeSnapshot{
		Transport:         BrowserBluetoothName,
		Connected:         connected,
		Ready:             b.isReadyLocked(),
		Supported:         b.supported,
		Stale:             stale,
		Status:            status,
		Message:           message,
		DeviceName:        b.deviceName,
		DeviceID:          b.deviceID,
		Protocol:          b.protocol,
		ClientID:          b.activeClientID,
		Pending:           len(b.pending),
		Inflight:          len(b.inflight),
		LastSeenAgeMillis: lastSeenAge,
		LastError:         b.lastError,
		LastAck:           cloneAckPtrValue(b.lastAck),
	}
}

func (b *BrowserBluetoothBridge) messageOrDefaultLocked() string {
	if strings.TrimSpace(b.message) != "" {
		return b.message
	}
	return "Handy Bluetooth is not connected."
}

func (b *BrowserBluetoothBridge) removeCommandLocked(commandID string) {
	next := b.pending[:0]
	for _, command := range b.pending {
		if command.ID != commandID {
			next = append(next, command)
		}
	}
	b.pending = next
	delete(b.inflight, commandID)
}

func (b *BrowserBluetoothBridge) failAllLocked(status string, message string) {
	for _, command := range b.pending {
		b.acks[command.ID] = b.failureAck(command.ID, status, message, 0)
	}
	b.pending = nil
	for commandID := range b.inflight {
		b.acks[commandID] = b.failureAck(commandID, status, message, 0)
		delete(b.inflight, commandID)
	}
}

func (b *BrowserBluetoothBridge) broadcastLocked() {
	close(b.notify)
	b.notify = make(chan struct{})
}

func safeShortString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func minDuration(a time.Duration, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func cloneBridgeCommands(commands []BrowserBluetoothBridgeCommand) []BrowserBluetoothBridgeCommand {
	clones := make([]BrowserBluetoothBridgeCommand, len(commands))
	for index, command := range commands {
		clones[index] = cloneBridgeCommand(command)
	}
	return clones
}

func cloneBridgeCommand(command BrowserBluetoothBridgeCommand) BrowserBluetoothBridgeCommand {
	command.Body = cloneBridgeBody(command.Body)
	return command
}

func cloneBridgeBody(body map[string]any) map[string]any {
	if body == nil {
		return nil
	}
	clone := make(map[string]any, len(body))
	for key, value := range body {
		clone[key] = cloneBridgeValue(value)
	}
	return clone
}

func cloneBridgeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneBridgeBody(typed)
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneBridgeValue(item)
		}
		return clone
	case []map[string]any:
		clone := make([]map[string]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneBridgeBody(item)
		}
		return clone
	default:
		return value
	}
}

func cloneAckPtr(ack BrowserBluetoothBridgeAck) *BrowserBluetoothBridgeAck {
	clone := ack
	clone.Response = cloneBridgeBody(ack.Response)
	return &clone
}

func cloneAckPtrValue(ack *BrowserBluetoothBridgeAck) *BrowserBluetoothBridgeAck {
	if ack == nil {
		return nil
	}
	return cloneAckPtr(*ack)
}
