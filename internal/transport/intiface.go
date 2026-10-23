package transport

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
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
	// Matched original-Handy FW4 trials favored fewer HDSP updates at 35%:
	// full sweeps, pace changes and Creative v2. This does not raise the hard
	// timing floor or change speed; the shared engine keeps faster reversals.
	handyIntifacePreferredInterval = 180 * time.Millisecond
)

var (
	errPacerSuperseded       = errors.New("Intiface pacer command superseded")
	errIntifaceResponseTime  = errors.New("Intiface response timed out")
	errIntifacePendingACKCap = errors.New("Intiface pending ACK capacity reached")
)

var _ Transport = (*Intiface)(nil)

type intifaceSelection struct {
	deviceIndex       uint32
	incarnation       uint64
	actuatorIndex     uint32
	timingGap         time.Duration
	resolutionPercent float64
	preferredInterval time.Duration
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

// Intiface is the linear owner: one selected LinearCmd actuator and its
// pacer, bound to a Buttplug v3 session. Session fields are promoted from the
// embedded session; selection and diagnostics share its lock.
type Intiface struct {
	*intifaceSession

	stopMu    sync.RWMutex
	ownerStop context.CancelFunc
	selection intifaceSelection
	selected  bool
	diagnosis TransportDiagnostics

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

// NewIntiface validates options and returns a disconnected Intiface owner
// with its own session.
func NewIntiface(options IntifaceOptions) (*Intiface, error) {
	session, err := newIntifaceSession(options)
	if err != nil {
		return nil, err
	}
	return newIntifaceLinear(session), nil
}

func newIntifaceLinear(session *intifaceSession) *Intiface {
	owner := &Intiface{
		intifaceSession: session,
		paceNotify:      make(chan struct{}, 1),
		window:          StrokeWindowCommand{MinPercent: 0, MaxPercent: 100},
		diagnosis: TransportDiagnostics{
			Name:          intifaceTransportName,
			PlaybackState: "idle",
		},
	}
	session.bind(owner)
	return owner
}

// Connect opens the owner's session; the linear pacer starts once it is ready.
func (i *Intiface) Connect(ctx context.Context) error {
	return i.intifaceSession.Connect(ctx)
}

func (i *Intiface) sessionReady(sessionCtx context.Context) {
	ownerCtx, cancel := context.WithCancel(sessionCtx)
	i.mu.Lock()
	i.ownerStop = cancel
	i.mu.Unlock()
	i.wg.Add(1)
	go i.paceLoop(ownerCtx)
}

// paceAlive is always true: the linear pacer is deadline-driven and reports
// its own starvation.
func (i *Intiface) paceAlive(time.Time) bool {
	return true
}

// invalidateForEmergency ends playback before the session's StopAllDevices,
// so no paced LinearCmd from this generation can follow it.
func (i *Intiface) invalidateForEmergency() {
	i.cancelPlayback("idle")
}

// Close stops pacing, attempts a device stop, and tears down the session.
func (i *Intiface) Close() error {
	return i.intifaceSession.Close()
}

func (i *Intiface) startPacedRequest(ctx context.Context, fields map[string]any, generation uint64, paced intifacePacedRequest) (intifaceRequest, error) {
	return i.startRequestFor(ctx, i, "LinearCmd", fields, generation, &paced)
}

func (i *Intiface) admitWrite(kind string, fields map[string]any, generation uint64, paced *intifacePacedRequest) (intifaceWriteAdmission, error) {
	return i.admitRequest(kind, fields, generation, paced)
}

func (i *Intiface) releaseWrite(reserved bool) {
	i.releasePendingACK(reserved)
}

func (i *Intiface) sessionConnectedLocked() {
	i.diagnosis.Connected = true
	i.diagnosis.PlaybackState = "idle"
}

func (i *Intiface) sessionFailedLocked(err error) func() {
	i.diagnosis.Connected = false
	i.diagnosis.PlaybackState = "stale"
	i.diagnosis.LastError = err.Error()
	return func() { i.cancelPlayback("stale") }
}

func (i *Intiface) deviceRemovedLocked(deviceIndex uint32) func() {
	if !i.selected || i.selection.deviceIndex != deviceIndex {
		return nil
	}
	i.selected = false
	return func() { i.cancelPlayback("stale") }
}

func (i *Intiface) invalidateForClose() {
	i.paceMu.Lock()
	i.invalidatePlaybackLocked(true)
	i.paceMu.Unlock()
	i.signalPacer()
}

func (i *Intiface) stopForClose(connected bool) error {
	i.mu.Lock()
	selected := i.selected
	i.mu.Unlock()
	if !connected || !selected {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := i.Stop(ctx, StopCommand{Reason: "owner_close"})
	return err
}

func (i *Intiface) closedLocked() {
	i.diagnosis.Connected = false
}

// admitRequest admits one of this owner's writes under the session writer
// lock: a superseded playback generation never reaches the wire, and paced
// LinearCmd writes reserve an acknowledgement slot.
func (i *Intiface) admitRequest(kind string, fields map[string]any, generation uint64, paced *intifacePacedRequest) (intifaceWriteAdmission, error) {
	admission := intifaceWriteAdmission{}
	if generation == 0 {
		return admission, nil
	}
	i.paceMu.Lock()
	defer i.paceMu.Unlock()
	if !i.playing || i.generation != generation {
		return admission, errPacerSuperseded
	}
	if paced != nil {
		var err error
		admission, err = i.preparePacedFieldsLocked(fields, *paced)
		if err != nil {
			return admission, err
		}
	}
	if kind != "LinearCmd" {
		return admission, nil
	}
	if i.pendingACKs >= maxIntifacePendingACKs {
		return admission, errIntifacePendingACKCap
	}
	i.pendingACKs++
	admission.linearReserved = true
	return admission, nil
}

func (i *Intiface) preparePacedFieldsLocked(fields map[string]any, paced intifacePacedRequest) (intifaceWriteAdmission, error) {
	admission := intifaceWriteAdmission{wireDuration: paced.originalDuration}
	var err error
	if paced.enforceSchedule {
		admission.lateness, admission.wireDuration, err = decideIntifaceLiveDuration(
			time.Now(), paced.scheduledAt, paced.scheduledEnd, paced.originalDuration, paced.minimumDuration,
		)
		if err == nil {
			admission.latestWrite, err = latestIntifaceWriteTime(paced)
		}
	}
	vectors, ok := fields["Vectors"].([]map[string]any)
	if !ok || len(vectors) != 1 {
		return admission, &intifacePacerError{category: "invalid_dispatch"}
	}
	if err != nil {
		return admission, err
	}
	vectors[0]["Position"] = projectIntifacePosition(paced.position, i.window)
	if paced.enforceSchedule {
		vectors[0]["Duration"] = admission.wireDuration.Milliseconds()
	}
	return admission, nil
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

// SelectDevice selects one discovered device and one of its linear actuators
// without commanding it. Production selection uses Select, which also stops
// the device first.
func (i *Intiface) SelectDevice(deviceIndex, actuatorIndex uint32) error {
	i.stopMu.Lock()
	defer i.stopMu.Unlock()
	if err := i.checkSelectable(); err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	selection, err := i.prepareSelectionLocked(deviceIndex, actuatorIndex)
	if err != nil {
		return err
	}
	if err := i.claimLocked(i, deviceIndex); err != nil {
		return err
	}
	return i.commitSelectionLocked(selection)
}

// Select chooses a linear actuator for playback. The device is claimed for
// this owner, and both the previous and the new device are stopped before
// the selection is ready, so neither keeps moving from an earlier command.
func (i *Intiface) Select(ctx context.Context, deviceIndex, actuatorIndex uint32) error {
	i.stopMu.Lock()
	err := i.checkSelectable()
	i.mu.Lock()
	var selection intifaceSelection
	if err == nil {
		selection, err = i.prepareSelectionLocked(deviceIndex, actuatorIndex)
	}
	if err == nil {
		err = i.claimLocked(i, deviceIndex)
	}
	previous, hadPrevious := i.selection.deviceIndex, i.selected
	i.mu.Unlock()
	i.stopMu.Unlock()
	if err != nil {
		return err
	}
	targets := []uint32{deviceIndex}
	if hadPrevious && previous != deviceIndex {
		targets = append(targets, previous)
	}
	for _, target := range targets {
		stopCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		err = errors.Join(err, i.requestOK(stopCtx, "StopDeviceCmd", map[string]any{"DeviceIndex": target}))
		cancel()
	}

	i.stopMu.Lock()
	defer i.stopMu.Unlock()
	if err == nil {
		err = i.checkSelectable()
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if err == nil {
		err = i.commitSelectionLocked(selection)
	}
	if err != nil {
		if !hadPrevious || previous != deviceIndex {
			i.releaseClaimLocked(i, deviceIndex)
		}
		return fmt.Errorf("Intiface device %d could not be prepared: %w", deviceIndex, err)
	}
	return nil
}

// Release stops and forgets the selected actuator while the session stays
// open for other outputs. A Stop that cannot be confirmed fails the session,
// because the server then stops its devices.
func (i *Intiface) Release(ctx context.Context) error {
	i.cancelPlayback("idle")
	i.mu.Lock()
	selection, selected, connected := i.selection, i.selected, i.connected
	if selected {
		i.selected = false
		i.releaseClaimLocked(i, selection.deviceIndex)
	}
	i.mu.Unlock()
	if !selected || !connected {
		return nil
	}
	stopCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if err := i.requestOK(stopCtx, "StopDeviceCmd", map[string]any{"DeviceIndex": selection.deviceIndex}); err != nil {
		i.setSessionFailure(fmt.Errorf("Intiface Stop could not be confirmed: %w", err))
		return err
	}
	return nil
}

func (i *Intiface) checkSelectable() error {
	if i.motionAdmissionClosed() {
		return errors.New("Intiface owner is closed")
	}
	i.paceMu.Lock()
	playing := i.playing
	i.paceMu.Unlock()
	if playing {
		return errors.New("cannot change Intiface device selection during playback")
	}
	return nil
}

func (i *Intiface) prepareSelectionLocked(deviceIndex, actuatorIndex uint32) (intifaceSelection, error) {
	device, ok := i.devices[deviceIndex]
	if !ok {
		return intifaceSelection{}, fmt.Errorf("Intiface device %d is not available", deviceIndex)
	}
	incarnation, _ := i.incarnationLocked(deviceIndex)
	for _, actuator := range device.LinearActuators {
		if actuator.Index != actuatorIndex {
			continue
		}
		if actuator.StepCount == 0 {
			return intifaceSelection{}, fmt.Errorf("Intiface device %d linear actuator %d advertises zero steps", deviceIndex, actuatorIndex)
		}
		timingGap := time.Duration(device.DeviceMessageTimingGapMillis) * time.Millisecond
		if timingGap > maxIntifaceDeviceTimingGap {
			return intifaceSelection{}, fmt.Errorf("Intiface device %d timing gap %dms exceeds the supported %dms limit", deviceIndex, device.DeviceMessageTimingGapMillis, maxIntifaceDeviceTimingGap.Milliseconds())
		}
		var preferredInterval time.Duration
		// Buttplug's protocol name, independent of its user display-name field.
		// Other linear devices keep their existing sampling behavior.
		if device.DeviceName == "The Handy (FW4+)" {
			preferredInterval = handyIntifacePreferredInterval
		}
		return intifaceSelection{
			deviceIndex:       deviceIndex,
			incarnation:       incarnation,
			actuatorIndex:     actuatorIndex,
			timingGap:         timingGap,
			resolutionPercent: 100 / float64(actuator.StepCount),
			preferredInterval: preferredInterval,
		}, nil
	}
	return intifaceSelection{}, fmt.Errorf("Intiface device %d has no linear actuator %d", deviceIndex, actuatorIndex)
}

// commitSelectionLocked binds the selection to the device incarnation it was
// prepared from and returns any other device this owner held.
func (i *Intiface) commitSelectionLocked(selection intifaceSelection) error {
	if incarnation, ok := i.incarnationLocked(selection.deviceIndex); !ok || incarnation != selection.incarnation {
		return errors.New("selected Intiface device is stale")
	}
	if i.selected && i.selection.deviceIndex != selection.deviceIndex {
		i.releaseClaimLocked(i, i.selection.deviceIndex)
	}
	i.selection = selection
	i.selected = true
	return nil
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
		PreferredPointInterval:      i.selection.preferredInterval,
	}
}

func (i *Intiface) motionAdmissionClosed() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.closed || !i.connected
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
	if incarnation, ok := i.incarnationLocked(i.selection.deviceIndex); !ok || incarnation != i.selection.incarnation {
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
