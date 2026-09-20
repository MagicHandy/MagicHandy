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
	defaultIntifaceAddress         = "ws://127.0.0.1:12345"
	defaultIntifaceClientName      = "MagicHandy"
	defaultIntifaceQueueCapacity   = 64
	defaultIntifaceResponseTime    = 650 * time.Millisecond
	maxIntifacePendingACKs         = 8
	maxIntifaceRecentDispatches    = 32
	intifaceTransportName          = "intiface_buttplug_v3"
	intifaceLateTolerance          = 100 * time.Millisecond
	intifaceWriteTimeout           = 500 * time.Millisecond
	intifaceAnchorDuration         = 250 * time.Millisecond
	maxIntifaceDeviceTimingGap     = 5 * time.Second
	minIntifaceTimingGapMargin     = 10 * time.Millisecond
	maxIntifaceTimingGapMargin     = 50 * time.Millisecond
	minimumIntifaceSegmentInterval = 50 * time.Millisecond
)

var (
	errPacerSuperseded       = errors.New("Intiface pacer command superseded")
	errIntifaceResponseTime  = errors.New("Intiface response timed out")
	errIntifacePendingACKCap = errors.New("Intiface pending ACK capacity reached")
)

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
	DeviceIndex                  uint32                   `json:"device_index"`
	DeviceName                   string                   `json:"device_name"`
	DeviceMessageTimingGapMillis uint32                   `json:"device_message_timing_gap_ms"`
	LinearActuators              []IntifaceLinearActuator `json:"linear_actuators"`
}

// IntifaceDispatchStatus is a bounded, protocol-safe paced-dispatch record.
type IntifaceDispatchStatus struct {
	DeviceIndex                 uint32 `json:"device_index"`
	ActuatorIndex               uint32 `json:"actuator_index"`
	StartupAnchor               bool   `json:"startup_anchor,omitempty"`
	RelativeScheduledTimeMillis int64  `json:"relative_scheduled_time_ms"`
	ActualSendTime              string `json:"actual_send_time"`
	LatenessMillis              int64  `json:"lateness_ms"`
	EffectiveDurationMillis     int64  `json:"effective_duration_ms"`
	ACKLatencyMillis            *int64 `json:"ack_latency_ms,omitempty"`
	Status                      string `json:"status"`
}

// IntifaceStatus is a safe connection, selection, and pacer snapshot.
type IntifaceStatus struct {
	Connected                 bool                     `json:"connected"`
	Scanning                  bool                     `json:"scanning"`
	PlaybackState             string                   `json:"playback_state"`
	MaxPingTimeMillis         int64                    `json:"max_ping_time_ms"`
	QueueDepth                int                      `json:"queue_depth"`
	QueueCoverageMillis       int64                    `json:"queue_coverage_ms"`
	PendingACKs               int                      `json:"pending_acks"`
	LinearSentCount           uint64                   `json:"linear_sent_count"`
	LinearACKedCount          uint64                   `json:"linear_acked_count"`
	LinearRejectedCount       uint64                   `json:"linear_rejected_count"`
	LinearTimeoutCount        uint64                   `json:"linear_timeout_count"`
	LastACKLatencyMillis      int64                    `json:"last_ack_latency_ms"`
	MaxACKLatencyMillis       int64                    `json:"max_ack_latency_ms"`
	LastSendLatenessMillis    int64                    `json:"last_send_lateness_ms"`
	MaxSendLatenessMillis     int64                    `json:"max_send_lateness_ms"`
	CoalescedSegments         uint64                   `json:"coalesced_segments"`
	RecentDispatchesDropped   uint64                   `json:"recent_dispatches_dropped"`
	LastWireDurationMillis    int64                    `json:"last_wire_duration_ms"`
	SelectedResolutionPercent float64                  `json:"selected_resolution_percent"`
	LastPacerFailure          string                   `json:"last_pacer_failure,omitempty"`
	RecentDispatches          []IntifaceDispatchStatus `json:"recent_dispatches"`
	SelectedDeviceIndex       *uint32                  `json:"selected_device_index,omitempty"`
	SelectedActuatorIndex     *uint32                  `json:"selected_actuator_index,omitempty"`
	Devices                   []IntifaceDevice         `json:"devices"`
}

type intifaceSelection struct {
	deviceIndex       uint32
	actuatorIndex     uint32
	timingGap         time.Duration
	resolutionPercent float64
}

func (s intifaceSelection) minimumPointInterval() time.Duration {
	if s.timingGap <= 0 {
		return 0
	}
	margin := s.timingGap / 10
	if margin < minIntifaceTimingGapMargin {
		margin = minIntifaceTimingGapMargin
	}
	if margin > maxIntifaceTimingGapMargin {
		margin = maxIntifaceTimingGapMargin
	}
	interval := s.timingGap + margin
	return ((interval + time.Millisecond - 1) / time.Millisecond) * time.Millisecond
}

type intifaceSegment struct {
	startMillis int64
	duration    int64
	position    float64
}

type intifaceAnchor struct {
	timeMillis int64
	position   float64
}

type intifaceWaiter struct {
	response chan buttplugMessage
	kind     string
}

type intifaceRequest struct {
	id           uint32
	waiter       *intifaceWaiter
	sessionCtx   context.Context
	writtenAt    time.Time
	linear       bool
	lateness     time.Duration
	wireDuration time.Duration
}

type intifaceWriteAdmission struct {
	linearReserved bool
	lateness       time.Duration
	wireDuration   time.Duration
	latestWrite    time.Time
}

type intifacePacedRequest struct {
	position         float64
	enforceSchedule  bool
	scheduledAt      time.Time
	scheduledEnd     time.Time
	originalDuration time.Duration
	minimumDuration  time.Duration
}

type intifacePacerError struct {
	category string
}

func (e *intifacePacerError) Error() string {
	return "Intiface pacer rejected unsafe dispatch: " + e.category
}

type intifaceDispatch struct {
	requestID uint32
	status    IntifaceDispatchStatus
}

// Intiface owns one persistent Buttplug v3 websocket session and linear pacer.
type Intiface struct {
	options IntifaceOptions

	lifecycleMu     sync.Mutex
	stopMu          sync.RWMutex
	workerMu        sync.Mutex
	workersClosed   bool
	mu              sync.Mutex
	writeMu         sync.Mutex
	conn            *websocket.Conn
	sessionCtx      context.Context
	sessionStop     context.CancelFunc
	started         bool
	closed          bool
	connected       bool
	scanning        bool
	maxPingTime     time.Duration
	nextID          uint32
	waiters         map[uint32]*intifaceWaiter
	devices         map[uint32]IntifaceDevice
	selection       intifaceSelection
	selected        bool
	diagnosis       TransportDiagnostics
	wg              sync.WaitGroup
	responseTimeout time.Duration

	paceMu            sync.Mutex
	paceNotify        chan struct{}
	queue             []intifaceSegment
	streamID          string
	tail              *TimedPoint
	anchor            *intifaceAnchor
	playing           bool
	generation        uint64
	playBase          time.Time
	playOffset        time.Duration
	coverageEnd       time.Time
	playCtx           context.Context
	playStop          context.CancelFunc
	anchoring         bool
	anchorInFlight    bool
	anchorDone        chan error
	window            StrokeWindowCommand
	pendingACKs       int
	linearSent        uint64
	linearACKed       uint64
	linearRejected    uint64
	linearTimeouts    uint64
	lastACKLatency    time.Duration
	maxACKLatency     time.Duration
	lastSendLateness  time.Duration
	maxSendLateness   time.Duration
	coalescedSegments uint64
	lastWireDuration  time.Duration
	lastPacerFailure  string
	recentDispatches  []intifaceDispatch
	recentDropped     uint64
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
		options:         options,
		paceNotify:      make(chan struct{}, 1),
		waiters:         make(map[uint32]*intifaceWaiter),
		devices:         make(map[uint32]IntifaceDevice),
		responseTimeout: defaultIntifaceResponseTime,
		window:          StrokeWindowCommand{MinPercent: 0, MaxPercent: 100},
		diagnosis: TransportDiagnostics{
			Name:          intifaceTransportName,
			PlaybackState: "idle",
		},
	}, nil
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
		status.SelectedResolutionPercent = i.selection.resolutionPercent
	}
	i.mu.Unlock()
	i.paceMu.Lock()
	status.QueueDepth = len(i.queue)
	status.QueueCoverageMillis = i.queueCoverageLocked(time.Now()).Milliseconds()
	status.PendingACKs = i.pendingACKs
	status.LinearSentCount = i.linearSent
	status.LinearACKedCount = i.linearACKed
	status.LinearRejectedCount = i.linearRejected
	status.LinearTimeoutCount = i.linearTimeouts
	status.LastACKLatencyMillis = i.lastACKLatency.Milliseconds()
	status.MaxACKLatencyMillis = i.maxACKLatency.Milliseconds()
	status.LastSendLatenessMillis = i.lastSendLateness.Milliseconds()
	status.MaxSendLatenessMillis = i.maxSendLateness.Milliseconds()
	status.CoalescedSegments = i.coalescedSegments
	status.RecentDispatchesDropped = i.recentDropped
	status.LastWireDurationMillis = i.lastWireDuration.Milliseconds()
	status.LastPacerFailure = i.lastPacerFailure
	status.RecentDispatches = make([]IntifaceDispatchStatus, len(i.recentDispatches))
	for index := range i.recentDispatches {
		status.RecentDispatches[index] = i.recentDispatches[index].status
		if latency := i.recentDispatches[index].status.ACKLatencyMillis; latency != nil {
			latencyCopy := *latency
			status.RecentDispatches[index].ACKLatencyMillis = &latencyCopy
		}
	}
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
	if i.closed || !i.connected {
		i.mu.Unlock()
		return errors.New("Intiface owner closed while scanning started")
	}
	i.mu.Unlock()
	return nil
}

// StopScanning asks Intiface to end device discovery.
func (i *Intiface) StopScanning(ctx context.Context) error {
	if err := i.requestOK(ctx, "StopScanning", nil); err != nil {
		return err
	}
	return nil
}

// SelectDevice selects one discovered device and one of its linear actuators.
func (i *Intiface) SelectDevice(deviceIndex, actuatorIndex uint32) error {
	i.stopMu.Lock()
	defer i.stopMu.Unlock()
	if i.motionAdmissionClosed() {
		return errors.New("Intiface owner is closed")
	}
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
			if actuator.StepCount == 0 {
				return fmt.Errorf("Intiface device %d linear actuator %d advertises zero steps", deviceIndex, actuatorIndex)
			}
			timingGap := time.Duration(device.DeviceMessageTimingGapMillis) * time.Millisecond
			if timingGap > maxIntifaceDeviceTimingGap {
				return fmt.Errorf("Intiface device %d timing gap %dms exceeds the supported %dms limit", deviceIndex, device.DeviceMessageTimingGapMillis, maxIntifaceDeviceTimingGap.Milliseconds())
			}
			i.selection = intifaceSelection{
				deviceIndex:       deviceIndex,
				actuatorIndex:     actuatorIndex,
				timingGap:         timingGap,
				resolutionPercent: 100 / float64(actuator.StepCount),
			}
			i.selected = true
			return nil
		}
	}
	return fmt.Errorf("Intiface device %d has no linear actuator %d", deviceIndex, actuatorIndex)
}

// PlaybackStartTime reports the post-anchor stream origin for engine clock alignment.
func (i *Intiface) PlaybackStartTime() time.Time {
	i.paceMu.Lock()
	defer i.paceMu.Unlock()
	if i.playBase.IsZero() {
		return time.Time{}
	}
	return i.playBase.Add(i.playOffset)
}

// MotionTimingCapabilities returns the selected device's command timing floor.
func (i *Intiface) MotionTimingCapabilities() MotionTimingCapabilities {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.selected {
		return MotionTimingCapabilities{}
	}
	// LinearCmd is immediate, even when the device advertises no radio gap.
	// Tiny HSP-style segments leave insufficient host/bridge scheduling margin.
	return MotionTimingCapabilities{MinimumPointInterval: max(minimumIntifaceSegmentInterval, i.selection.minimumPointInterval())}
}

// MotionSamplingCapabilities reports the selected actuator's physical step
// resolution. The engine scales it back through the active stroke window
// before reducing semantic samples.
func (i *Intiface) MotionSamplingCapabilities() MotionSamplingCapabilities {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.selected {
		return MotionSamplingCapabilities{}
	}
	return MotionSamplingCapabilities{
		PositionResolutionPercent:   i.selection.resolutionPercent,
		ResolutionAfterStrokeWindow: true,
	}
}

func (i *Intiface) motionAdmissionClosed() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.closed || !i.connected
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
	i.playBase = time.Time{}
	i.playOffset = 0
	i.anchoring = false
	i.anchorInFlight = false
	i.finishAnchorLocked(errPacerSuperseded)
	if i.playStop != nil {
		i.playStop()
		i.playStop = nil
	}
	i.playCtx = nil
	if clearStream {
		i.streamID = ""
		i.tail = nil
		i.anchor = nil
	}
}

func (i *Intiface) finishAnchorLocked(err error) {
	if i.anchorDone == nil {
		return
	}
	i.anchorDone <- err
	i.anchorDone = nil
}

func (i *Intiface) stopSelectedDevice(ctx context.Context) error {
	selection, err := i.selectedDevice()
	if err != nil {
		return err
	}
	stopCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	err = i.requestOK(stopCtx, "StopDeviceCmd", map[string]any{"DeviceIndex": selection.deviceIndex})
	cancel()
	return err
}

func (i *Intiface) signalPacer() {
	select {
	case i.paceNotify <- struct{}{}:
	default:
	}
}

func (i *Intiface) releasePendingACK(reserved bool) {
	if !reserved {
		return
	}
	i.paceMu.Lock()
	if i.pendingACKs > 0 {
		i.pendingACKs--
	}
	i.paceMu.Unlock()
}

func (i *Intiface) queueCoverageLocked(now time.Time) time.Duration {
	if len(i.queue) == 0 {
		if i.playing {
			coverage := i.coverageEnd.Sub(now)
			if coverage > 0 {
				return coverage
			}
		}
		return 0
	}
	last := i.queue[len(i.queue)-1]
	if i.playing && !i.playBase.IsZero() {
		coverage := i.playBase.Add(time.Duration(last.startMillis+last.duration) * time.Millisecond).Sub(now)
		if coverage > 0 {
			return coverage
		}
		return 0
	}
	first := i.queue[0]
	coverage := time.Duration(last.startMillis+last.duration-first.startMillis) * time.Millisecond
	if coverage > 0 {
		return coverage
	}
	return 0
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
	projected := float64(window.MinPercent) + position*(float64(window.MaxPercent-window.MinPercent)/100)
	return projected / 100
}

func snapshotIntifacePosition(position float64, reverse bool) float64 {
	if reverse {
		return 100 - position
	}
	return position
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
