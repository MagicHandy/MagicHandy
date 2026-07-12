package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	defaultIntifaceAddress       = "ws://127.0.0.1:12345"
	defaultIntifaceClientName    = "MagicHandy"
	defaultIntifaceQueueCapacity = 2048
	intifaceTransportName        = "intiface_buttplug_v3"
	intifaceLateTolerance        = 100 * time.Millisecond
)

var errPacerSuperseded = errors.New("Intiface pacer command superseded")

var _ Transport = (*Intiface)(nil)

// IntifaceOptions configures the connection to a user-owned Intiface server.
type IntifaceOptions struct {
	Address       string
	ClientName    string
	QueueCapacity int
	HTTPClient    *http.Client
}

// IntifaceLinearActuator describes one selectable Buttplug LinearCmd feature.
type IntifaceLinearActuator struct {
	Index             uint32 `json:"index"`
	FeatureDescriptor string `json:"feature_descriptor,omitempty"`
	ActuatorType      string `json:"actuator_type,omitempty"`
	StepCount         uint32 `json:"step_count,omitempty"`
}

// IntifaceDevice is a safe snapshot of a discovered Buttplug device.
type IntifaceDevice struct {
	DeviceIndex     uint32                   `json:"device_index"`
	DeviceName      string                   `json:"device_name"`
	LinearActuators []IntifaceLinearActuator `json:"linear_actuators"`
}

// IntifaceStatus is a safe connection, selection, and pacer snapshot.
type IntifaceStatus struct {
	Connected             bool             `json:"connected"`
	Scanning              bool             `json:"scanning"`
	PlaybackState         string           `json:"playback_state"`
	MaxPingTimeMillis     int64            `json:"max_ping_time_ms"`
	QueueDepth            int              `json:"queue_depth"`
	SelectedDeviceIndex   *uint32          `json:"selected_device_index,omitempty"`
	SelectedActuatorIndex *uint32          `json:"selected_actuator_index,omitempty"`
	Devices               []IntifaceDevice `json:"devices"`
}

type intifaceSelection struct {
	deviceIndex   uint32
	actuatorIndex uint32
}

type intifaceSegment struct {
	startMillis int64
	duration    int64
	position    float64
}

// Intiface owns one persistent Buttplug v3 websocket session and linear pacer.
type Intiface struct {
	options IntifaceOptions

	lifecycleMu sync.Mutex
	mu          sync.Mutex
	writeMu     sync.Mutex
	conn        *websocket.Conn
	sessionCtx  context.Context
	sessionStop context.CancelFunc
	started     bool
	closed      bool
	connected   bool
	scanning    bool
	maxPingTime time.Duration
	nextID      uint32
	waiters     map[uint32]chan buttplugMessage
	devices     map[uint32]IntifaceDevice
	selection   intifaceSelection
	selected    bool
	diagnosis   TransportDiagnostics
	wg          sync.WaitGroup

	paceMu      sync.Mutex
	paceNotify  chan struct{}
	queue       []intifaceSegment
	streamID    string
	tail        *TimedPoint
	playing     bool
	generation  uint64
	playBase    time.Time
	coverageEnd time.Time
	playCtx     context.Context
	playStop    context.CancelFunc
	window      StrokeWindowCommand
}

// NewIntiface validates options and returns a disconnected Intiface owner.
func NewIntiface(options IntifaceOptions) (*Intiface, error) {
	options.Address = strings.TrimSpace(options.Address)
	if options.Address == "" {
		options.Address = defaultIntifaceAddress
	}
	parsed, err := url.Parse(options.Address)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Host == "" {
		return nil, errors.New("Intiface address must be an absolute ws:// or wss:// URL")
	}
	options.ClientName = strings.TrimSpace(options.ClientName)
	if options.ClientName == "" {
		options.ClientName = defaultIntifaceClientName
	}
	if options.QueueCapacity == 0 {
		options.QueueCapacity = defaultIntifaceQueueCapacity
	}
	if options.QueueCapacity < 1 {
		return nil, errors.New("Intiface queue capacity must be positive")
	}

	return &Intiface{
		options:    options,
		paceNotify: make(chan struct{}, 1),
		waiters:    make(map[uint32]chan buttplugMessage),
		devices:    make(map[uint32]IntifaceDevice),
		window:     StrokeWindowCommand{MinPercent: 0, MaxPercent: 100},
		diagnosis: TransportDiagnostics{
			Name:          intifaceTransportName,
			PlaybackState: "idle",
		},
	}, nil
}

// Connect opens the websocket and completes the Buttplug v3 handshake and device list request.
func (i *Intiface) Connect(ctx context.Context) error {
	i.lifecycleMu.Lock()
	defer i.lifecycleMu.Unlock()

	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return errors.New("Intiface owner is closed")
	}
	if i.started {
		i.mu.Unlock()
		return errors.New("Intiface connection has already been started")
	}
	i.started = true
	i.mu.Unlock()

	conn, response, err := websocket.Dial(ctx, i.options.Address, &websocket.DialOptions{HTTPClient: i.options.HTTPClient})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		i.setSessionFailure(fmt.Errorf("connect to Intiface: %w", err))
		return err
	}
	conn.SetReadLimit(1 << 20)
	sessionCtx, sessionStop := context.WithCancel(context.Background())
	i.mu.Lock()
	i.conn = conn
	i.sessionCtx = sessionCtx
	i.sessionStop = sessionStop
	i.connected = true
	i.diagnosis.Connected = true
	i.diagnosis.PlaybackState = "idle"
	i.mu.Unlock()

	i.wg.Add(1)
	go i.readLoop(sessionCtx, conn)

	info, devices, err := i.handshake(ctx)
	if err != nil {
		i.abortConnect(err)
		return err
	}
	i.mu.Lock()
	i.maxPingTime = time.Duration(info.MaxPingTime) * time.Millisecond
	i.mu.Unlock()
	i.replaceDevices(devices)

	i.wg.Add(1)
	go i.paceLoop(sessionCtx)
	if info.MaxPingTime > 0 {
		i.wg.Add(1)
		go i.pingLoop(sessionCtx, time.Duration(info.MaxPingTime)*time.Millisecond)
	}
	return nil
}

func (i *Intiface) handshake(ctx context.Context) (buttplugServerInfo, []buttplugDevice, error) {
	message, err := i.request(ctx, "RequestServerInfo", map[string]any{
		"ClientName":     i.options.ClientName,
		"MessageVersion": buttplugMessageVersion,
	})
	if err != nil {
		return buttplugServerInfo{}, nil, err
	}
	if message.kind != "ServerInfo" {
		return buttplugServerInfo{}, nil, fmt.Errorf("expected Buttplug ServerInfo, received %s", message.kind)
	}
	var info buttplugServerInfo
	if err := json.Unmarshal(message.payload, &info); err != nil {
		return buttplugServerInfo{}, nil, err
	}
	if info.MessageVersion != buttplugMessageVersion {
		return buttplugServerInfo{}, nil, fmt.Errorf("Intiface negotiated Buttplug message version %d, want %d", info.MessageVersion, buttplugMessageVersion)
	}
	if info.MaxPingTime < 0 {
		return buttplugServerInfo{}, nil, errors.New("Intiface MaxPingTime must be non-negative")
	}

	message, err = i.request(ctx, "RequestDeviceList", nil)
	if err != nil {
		return buttplugServerInfo{}, nil, err
	}
	if message.kind != "DeviceList" {
		return buttplugServerInfo{}, nil, fmt.Errorf("expected Buttplug DeviceList, received %s", message.kind)
	}
	var list buttplugDeviceList
	if err := json.Unmarshal(message.payload, &list); err != nil {
		return buttplugServerInfo{}, nil, err
	}
	return info, list.Devices, nil
}

// Close stops pacing, attempts a device stop, and tears down all session goroutines.
func (i *Intiface) Close() error {
	i.lifecycleMu.Lock()
	defer i.lifecycleMu.Unlock()

	i.paceMu.Lock()
	i.invalidatePlaybackLocked(true)
	i.paceMu.Unlock()
	i.signalPacer()

	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return nil
	}
	i.closed = true
	connected := i.connected
	selection, selected := i.selection, i.selected
	stopSession := i.sessionStop
	conn := i.conn
	i.mu.Unlock()

	if connected && selected {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_ = i.requestOK(ctx, "StopDeviceCmd", map[string]any{"DeviceIndex": selection.deviceIndex})
		cancel()
	}
	if stopSession != nil {
		stopSession()
	}
	var closeErr error
	if conn != nil && connected {
		closeErr = conn.CloseNow()
	}
	i.wg.Wait()
	i.mu.Lock()
	i.connected = false
	i.diagnosis.Connected = false
	i.mu.Unlock()
	return closeErr
}

// Status returns a safe device, selection, and playback snapshot.
func (i *Intiface) Status() IntifaceStatus {
	i.mu.Lock()
	status := IntifaceStatus{
		Connected:         i.connected,
		Scanning:          i.scanning,
		PlaybackState:     i.diagnosis.PlaybackState,
		MaxPingTimeMillis: i.maxPingTime.Milliseconds(),
		Devices:           cloneIntifaceDevicesLocked(i.devices),
	}
	if i.selected {
		deviceIndex := i.selection.deviceIndex
		actuatorIndex := i.selection.actuatorIndex
		status.SelectedDeviceIndex = &deviceIndex
		status.SelectedActuatorIndex = &actuatorIndex
	}
	i.mu.Unlock()
	i.paceMu.Lock()
	status.QueueDepth = len(i.queue)
	i.paceMu.Unlock()
	return status
}

// Devices returns the current safe discovered-device snapshot.
func (i *Intiface) Devices() []IntifaceDevice {
	i.mu.Lock()
	defer i.mu.Unlock()
	return cloneIntifaceDevicesLocked(i.devices)
}

// StartScanning asks Intiface to begin device discovery.
func (i *Intiface) StartScanning(ctx context.Context) error {
	if err := i.requestOK(ctx, "StartScanning", nil); err != nil {
		return err
	}
	i.mu.Lock()
	i.scanning = true
	i.mu.Unlock()
	return nil
}

// StopScanning asks Intiface to end device discovery.
func (i *Intiface) StopScanning(ctx context.Context) error {
	if err := i.requestOK(ctx, "StopScanning", nil); err != nil {
		return err
	}
	i.mu.Lock()
	i.scanning = false
	i.mu.Unlock()
	return nil
}

// SelectDevice selects one discovered device and one of its linear actuators.
func (i *Intiface) SelectDevice(deviceIndex, actuatorIndex uint32) error {
	i.paceMu.Lock()
	playing := i.playing
	i.paceMu.Unlock()
	if playing {
		return errors.New("cannot change Intiface device selection during playback")
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	device, ok := i.devices[deviceIndex]
	if !ok {
		return fmt.Errorf("Intiface device %d is not available", deviceIndex)
	}
	for _, actuator := range device.LinearActuators {
		if actuator.Index == actuatorIndex {
			i.selection = intifaceSelection{deviceIndex: deviceIndex, actuatorIndex: actuatorIndex}
			i.selected = true
			return nil
		}
	}
	return fmt.Errorf("Intiface device %d has no linear actuator %d", deviceIndex, actuatorIndex)
}

func (i *Intiface) readLoop(ctx context.Context, conn *websocket.Conn) {
	defer i.wg.Done()
	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				i.setSessionFailure(fmt.Errorf("read Intiface websocket: %w", err))
			}
			return
		}
		if messageType != websocket.MessageText {
			i.setSessionFailure(errors.New("Intiface sent a non-text websocket message"))
			return
		}
		messages, err := decodeButtplugMessages(data)
		if err != nil {
			i.setSessionFailure(err)
			return
		}
		for _, message := range messages {
			switch message.kind {
			case "DeviceAdded":
				i.handleDeviceAdded(message.payload)
			case "DeviceRemoved":
				i.handleDeviceRemoved(message.payload)
			case "ScanningFinished":
				i.mu.Lock()
				i.scanning = false
				i.mu.Unlock()
			default:
				i.routeResponse(message)
			}
		}
	}
}

func (i *Intiface) pingLoop(ctx context.Context, maxPingTime time.Duration) {
	defer i.wg.Done()
	interval := maxPingTime / 2
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			timeout := maxPingTime / 3
			if timeout <= 0 {
				timeout = time.Millisecond
			}
			pingCtx, cancel := context.WithTimeout(ctx, timeout)
			err := i.requestOK(pingCtx, "Ping", nil)
			cancel()
			if err != nil && ctx.Err() == nil {
				i.setSessionFailure(fmt.Errorf("Intiface ping failed: %w", err))
				return
			}
		}
	}
}

func (i *Intiface) request(ctx context.Context, kind string, fields map[string]any) (buttplugMessage, error) {
	return i.requestGuarded(ctx, kind, fields, 0)
}

func (i *Intiface) requestOK(ctx context.Context, kind string, fields map[string]any) error {
	message, err := i.request(ctx, kind, fields)
	if err != nil {
		return err
	}
	if message.kind != "Ok" {
		return fmt.Errorf("expected Buttplug Ok for %s, received %s", kind, message.kind)
	}
	return nil
}

func (i *Intiface) requestGuarded(ctx context.Context, kind string, fields map[string]any, generation uint64) (buttplugMessage, error) {
	i.mu.Lock()
	if !i.connected || i.conn == nil {
		i.mu.Unlock()
		return buttplugMessage{}, errors.New("Intiface connection is stale")
	}
	i.nextID++
	if i.nextID == 0 {
		i.nextID++
	}
	id := i.nextID
	waiter := make(chan buttplugMessage, 1)
	i.waiters[id] = waiter
	conn := i.conn
	sessionCtx := i.sessionCtx
	i.mu.Unlock()

	body := make(map[string]any, len(fields)+1)
	body["Id"] = id
	for key, value := range fields {
		body[key] = value
	}
	data, err := json.Marshal([]map[string]any{{kind: body}})
	if err != nil {
		i.removeWaiter(id)
		return buttplugMessage{}, err
	}

	i.writeMu.Lock()
	if generation != 0 {
		i.paceMu.Lock()
		active := i.playing && i.generation == generation
		i.paceMu.Unlock()
		if !active {
			i.writeMu.Unlock()
			i.removeWaiter(id)
			return buttplugMessage{}, errPacerSuperseded
		}
	}
	err = conn.Write(ctx, websocket.MessageText, data)
	i.writeMu.Unlock()
	if err != nil {
		i.removeWaiter(id)
		if sessionCtx.Err() == nil && !errors.Is(err, context.Canceled) {
			i.setSessionFailure(err)
		}
		return buttplugMessage{}, err
	}

	select {
	case message := <-waiter:
		i.removeWaiter(id)
		if message.kind == "Error" {
			var protocolError buttplugError
			_ = json.Unmarshal(message.payload, &protocolError)
			return message, fmt.Errorf("Intiface rejected %s: %s", kind, protocolError.ErrorMessage)
		}
		return message, nil
	case <-ctx.Done():
		i.removeWaiter(id)
		return buttplugMessage{}, ctx.Err()
	case <-sessionCtx.Done():
		i.removeWaiter(id)
		return buttplugMessage{}, errors.New("Intiface connection is stale")
	}
}

func (i *Intiface) routeResponse(message buttplugMessage) {
	if message.id == 0 {
		return
	}
	i.mu.Lock()
	waiter := i.waiters[message.id]
	i.mu.Unlock()
	if waiter != nil {
		select {
		case waiter <- message:
		default:
		}
	}
}

func (i *Intiface) removeWaiter(id uint32) {
	i.mu.Lock()
	delete(i.waiters, id)
	i.mu.Unlock()
}

func (i *Intiface) handleDeviceAdded(payload json.RawMessage) {
	var device buttplugDevice
	if json.Unmarshal(payload, &device) != nil {
		return
	}
	i.mu.Lock()
	i.devices[device.DeviceIndex] = intifaceDeviceFromProtocol(device)
	i.mu.Unlock()
}

func (i *Intiface) handleDeviceRemoved(payload json.RawMessage) {
	var removed buttplugDeviceRemoved
	if json.Unmarshal(payload, &removed) != nil {
		return
	}
	i.mu.Lock()
	delete(i.devices, removed.DeviceIndex)
	selected := i.selected && i.selection.deviceIndex == removed.DeviceIndex
	if selected {
		i.selected = false
	}
	i.mu.Unlock()
	if selected {
		i.cancelPlayback("stale")
	}
}

func (i *Intiface) replaceDevices(devices []buttplugDevice) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.devices = make(map[uint32]IntifaceDevice, len(devices))
	for _, device := range devices {
		i.devices[device.DeviceIndex] = intifaceDeviceFromProtocol(device)
	}
}

func (i *Intiface) selectedDevice() (intifaceSelection, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.connected {
		return intifaceSelection{}, errors.New("Intiface connection is stale")
	}
	if !i.selected {
		return intifaceSelection{}, errors.New("an Intiface linear actuator must be selected")
	}
	if _, ok := i.devices[i.selection.deviceIndex]; !ok {
		return intifaceSelection{}, errors.New("selected Intiface device is stale")
	}
	return i.selection, nil
}

func (i *Intiface) setSessionFailure(err error) {
	i.mu.Lock()
	wasConnected := i.connected
	i.connected = false
	i.scanning = false
	i.diagnosis.Connected = false
	i.diagnosis.PlaybackState = "stale"
	i.diagnosis.LastError = err.Error()
	stop := i.sessionStop
	conn := i.conn
	i.mu.Unlock()
	if !wasConnected {
		return
	}
	if stop != nil {
		stop()
	}
	if conn != nil {
		_ = conn.CloseNow()
	}
	i.cancelPlayback("stale")
}

func (i *Intiface) abortConnect(err error) {
	i.setSessionFailure(err)
	i.wg.Wait()
}

func (i *Intiface) cancelPlayback(state string) {
	i.paceMu.Lock()
	i.invalidatePlaybackLocked(true)
	i.paceMu.Unlock()
	i.setPlaybackState(state)
	i.signalPacer()
}

func (i *Intiface) invalidatePlaybackLocked(clearStream bool) {
	i.generation++
	i.playing = false
	i.queue = nil
	i.coverageEnd = time.Time{}
	if i.playStop != nil {
		i.playStop()
		i.playStop = nil
	}
	i.playCtx = nil
	if clearStream {
		i.streamID = ""
		i.tail = nil
	}
}

func (i *Intiface) stopSelectedDevice(ctx context.Context) {
	selection, err := i.selectedDevice()
	if err != nil {
		return
	}
	stopCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	_ = i.requestOK(stopCtx, "StopDeviceCmd", map[string]any{"DeviceIndex": selection.deviceIndex})
	cancel()
}

func (i *Intiface) signalPacer() {
	select {
	case i.paceNotify <- struct{}{}:
	default:
	}
}

func (i *Intiface) waitForPacer(ctx context.Context, delay time.Duration) bool {
	if ctx == nil {
		return true
	}
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return false
		case <-i.paceNotify:
			return true
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-i.paceNotify:
		return true
	case <-timer.C:
		return false
	}
}

func (i *Intiface) completeCommand(command Command, status string, started time.Time, err error) CommandResult {
	i.mu.Lock()
	defer i.mu.Unlock()
	command.ID = fmt.Sprintf("intiface-%06d", i.diagnosis.CommandCount+1)
	command.IssuedAt = started.UTC().Format(time.RFC3339Nano)
	result := CommandResult{
		CommandID:     command.ID,
		Kind:          command.Kind,
		Transport:     intifaceTransportName,
		OK:            err == nil,
		Status:        status,
		LatencyMillis: time.Since(started).Milliseconds(),
		CompletedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err != nil {
		result.Error = err.Error()
		i.diagnosis.LastError = err.Error()
	} else {
		i.diagnosis.LastError = ""
	}
	i.diagnosis.Connected = i.connected
	i.diagnosis.CommandCount++
	i.diagnosis.LastLatencyMillis = result.LatencyMillis
	i.diagnosis.LastCommand = &command
	i.diagnosis.LastResult = &result
	return result
}

func (i *Intiface) setPlaybackState(state string) {
	i.mu.Lock()
	i.diagnosis.PlaybackState = state
	i.mu.Unlock()
}

func validateIntifacePoints(points []TimedPoint) error {
	if len(points) == 0 {
		return errors.New("Intiface append requires at least one point")
	}
	for index, point := range points {
		if math.IsNaN(point.PositionPercent) || math.IsInf(point.PositionPercent, 0) || point.PositionPercent < 0 || point.PositionPercent > 100 {
			return fmt.Errorf("point %d position must be finite and between 0 and 100", index)
		}
		if point.TimeMillis < 0 {
			return fmt.Errorf("point %d time must be non-negative", index)
		}
		if point.TimeMillis > maxIntifaceScheduleMillis {
			return fmt.Errorf("point %d time is too large", index)
		}
		if index > 0 && point.TimeMillis <= points[index-1].TimeMillis {
			return fmt.Errorf("point %d time must be strictly increasing", index)
		}
		if index > 0 && point.TimeMillis-points[index-1].TimeMillis > int64(^uint32(0)) {
			return fmt.Errorf("point %d duration exceeds Buttplug LinearCmd bounds", index)
		}
	}
	return nil
}

const maxIntifaceScheduleMillis = int64((1<<63 - 1) / int64(time.Millisecond))

func projectIntifacePosition(position float64, window StrokeWindowCommand) float64 {
	if window.ReverseDirection {
		position = 100 - position
	}
	projected := float64(window.MinPercent) + position*(float64(window.MaxPercent-window.MinPercent)/100)
	return projected / 100
}

func cloneIntifaceDevicesLocked(devices map[uint32]IntifaceDevice) []IntifaceDevice {
	clones := make([]IntifaceDevice, 0, len(devices))
	for _, device := range devices {
		clone := device
		clone.LinearActuators = append([]IntifaceLinearActuator(nil), device.LinearActuators...)
		clones = append(clones, clone)
	}
	sort.Slice(clones, func(a, b int) bool { return clones[a].DeviceIndex < clones[b].DeviceIndex })
	return clones
}
