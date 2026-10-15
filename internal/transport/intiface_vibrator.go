package transport

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/companion"
	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

// Vibrator pacing and safety constants. The cadence is a starting ceiling,
// not a measured motor bandwidth.
const (
	vibratorTransportName = "intiface_vibrator_v3"
	vibratorTick          = 100 * time.Millisecond
	vibratorPreroll       = 120 * time.Millisecond
	vibratorStarveAfter   = 750 * time.Millisecond
	vibratorMinimumLead   = 500 * time.Millisecond
	vibratorMediaLead     = time.Second
	vibratorPruneMillis   = 1000
	vibratorTestDuration  = time.Second
	vibratorTestCeiling   = 0.25
	vibratorStopTimeout   = 500 * time.Millisecond
	// A pacer that has not run for this long while playing is stalled: the
	// watchdog stops the device and the session stops answering pings.
	vibratorStallLimit    = 750 * time.Millisecond
	vibratorWatchdogTick  = 250 * time.Millisecond
	vibratorActuatorType  = "Vibrate"
	maxVibratorOffsetMs   = 300
	vibratorTimingGapUnit = time.Millisecond
)

var (
	errVibratorControlStale = errors.New("the vibrator changed or was stopped; try again")
	errVibratorNotSelected  = errors.New("choose a vibrator in the connection manager first")
	errVibratorStale        = errors.New("the vibrator connection changed; start a new playback session")
	errVibratorPlaying      = errors.New("stop playback before testing or changing the vibrator")
	errVibratorSilent       = errors.New("raise the vibrator intensity before testing it")
	errVibratorNoMotors     = errors.New("this Intiface device has no vibration motors")
)

// VibratorConfig is the user's vibration settings. Levels are fractions.
type VibratorConfig struct {
	Profile      vibration.Profile
	Minimum      float64
	Ceiling      float64
	OffsetMillis int
}

// IntifaceVibratorMotor is one Vibrate actuator the vibrator drives.
type IntifaceVibratorMotor struct {
	Index             uint32 `json:"index"`
	FeatureDescriptor string `json:"feature_descriptor,omitempty"`
	StepCount         int    `json:"step_count"`
}

// IntifaceVibratorStatus is the vibrator's safe snapshot. Percentages are
// whole numbers; the effective ceiling is the highest level the motors can
// deliver at or below the ceiling.
type IntifaceVibratorStatus struct {
	Selected                bool                    `json:"selected"`
	DeviceIndex             *uint32                 `json:"device_index,omitempty"`
	DeviceName              string                  `json:"device_name,omitempty"`
	Motors                  []IntifaceVibratorMotor `json:"motors"`
	IntensityPercent        int                     `json:"intensity_percent"`
	CeilingPercent          int                     `json:"ceiling_percent"`
	EffectiveCeilingPercent int                     `json:"effective_ceiling_percent"`
	LevelPercent            int                     `json:"level_percent"`
	PlaybackState           string                  `json:"playback_state"`
	Testing                 bool                    `json:"testing"`
	CommandedCount          uint64                  `json:"commanded_count"`
	AcknowledgedCount       uint64                  `json:"acknowledged_count"`
	RejectedCount           uint64                  `json:"rejected_count"`
	TimeoutCount            uint64                  `json:"timeout_count"`
	LastError               string                  `json:"last_error,omitempty"`
}

type vibratorMotor struct {
	IntifaceVibratorMotor
	last int
}

// IntifaceVibrator drives one Buttplug device's Vibrate actuators from the
// shared motion frame through a conversion profile. It binds to the shared
// Intiface session; the model never controls its intensity.
type IntifaceVibrator struct {
	session *intifaceSession
	clock   func() time.Time
	tick    time.Duration
	wake    chan struct{}
	// manualPace leaves the pacer goroutine off so tests drive paceOnce on a
	// manual clock; the watchdog still runs.
	manualPace bool

	mu        sync.Mutex
	config    VibratorConfig
	intensity float64
	selection vibratorSelection
	epoch     uint64
	// control changes with every Stop, selection and session change: a raise
	// or test prepared before one of them is refused.
	control   uint64
	stream    vibratorStream
	inFlight  bool
	testUntil time.Time
	level     float64
	stats     vibratorStats
	diag      TransportDiagnostics

	// Read without the lock by the watchdog and the session's ping loop.
	lastPace     atomic.Int64
	playingFlag  atomic.Bool
	stalled      atomic.Bool
	watchedIndex atomic.Uint32
}

type vibratorSelection struct {
	selected    bool
	deviceIndex uint32
	incarnation uint64
	deviceName  string
	timingGap   time.Duration
	motors      []vibratorMotor
}

type vibratorStream struct {
	renderer   *vibration.Renderer
	timeline   companion.Timeline
	origin     companion.Clock
	coverage   companion.Coverage
	id         string
	playing    bool
	generation uint64
	state      string
}

type vibratorStats struct {
	commanded uint64
	acked     uint64
	rejected  uint64
	timeouts  uint64
	lastError string
}

func newIntifaceVibrator(session *intifaceSession) *IntifaceVibrator {
	vibrator := &IntifaceVibrator{
		session: session,
		clock:   time.Now,
		tick:    vibratorTick,
		wake:    make(chan struct{}, 1),
		config:  VibratorConfig{Profile: vibration.Profile{Follow: vibration.FollowSpeed, DepthEnd: vibration.DepthBase}, Ceiling: 0.4},
		stream: vibratorStream{
			renderer: vibration.NewRenderer(vibration.Profile{Follow: vibration.FollowSpeed, DepthEnd: vibration.DepthBase}),
			coverage: companion.NewCoverage(vibratorStarveAfter),
			state:    "idle",
		},
		diag: TransportDiagnostics{Name: vibratorTransportName, PlaybackState: "idle"},
	}
	session.bind(vibrator)
	return vibrator
}

// Configure applies the user's vibration settings. Lowering the ceiling
// lowers the live intensity at once.
func (v *IntifaceVibrator) Configure(config VibratorConfig) {
	config.Minimum = clampUnit(config.Minimum)
	config.Ceiling = clampUnit(config.Ceiling)
	config.OffsetMillis = max(-maxVibratorOffsetMs, min(maxVibratorOffsetMs, config.OffsetMillis))
	v.mu.Lock()
	v.config = config
	v.stream.renderer.SetProfile(config.Profile)
	v.intensity = math.Min(v.intensity, config.Ceiling)
	v.mu.Unlock()
	v.signal()
}

// SetIntensity sets the live intensity, capped by the ceiling, and returns
// it. Permission to raise belongs to the caller; a lower value takes effect
// on the next send, without waiting for the pacer's tick.
func (v *IntifaceVibrator) SetIntensity(fraction float64) float64 {
	v.mu.Lock()
	v.intensity = math.Min(clampUnit(fraction), v.config.Ceiling)
	intensity := v.intensity
	v.mu.Unlock()
	v.signal()
	return intensity
}

// ControlGeneration names the vibrator state a request was prepared against.
func (v *IntifaceVibrator) ControlGeneration() uint64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.control
}

// SetIntensityAt changes the live intensity for a request prepared at
// generation. Lowering always applies; a raise prepared before a Stop, a
// selection or a session change is refused.
func (v *IntifaceVibrator) SetIntensityAt(generation uint64, fraction float64) (float64, error) {
	v.mu.Lock()
	target := math.Min(clampUnit(fraction), v.config.Ceiling)
	if target > v.intensity && generation != v.control {
		v.mu.Unlock()
		return 0, errVibratorControlStale
	}
	if target > v.intensity && !v.selection.selected {
		v.mu.Unlock()
		return 0, errVibratorNotSelected
	}
	v.intensity = target
	v.signalLocked()
	v.mu.Unlock()
	return target, nil
}

// TestAt starts a test for a request prepared at generation.
func (v *IntifaceVibrator) TestAt(generation uint64) error {
	v.mu.Lock()
	stale := generation != v.control
	v.mu.Unlock()
	if stale {
		return errVibratorControlStale
	}
	return v.Test()
}

// Intensity reports the live intensity.
func (v *IntifaceVibrator) Intensity() float64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.intensity
}

// Test plays one second at the lower of the live intensity and 25% while
// motion is idle.
func (v *IntifaceVibrator) Test() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	switch {
	case !v.selection.selected:
		return errVibratorNotSelected
	case v.stream.playing:
		return errVibratorPlaying
	case v.intensity <= 0:
		return errVibratorSilent
	}
	v.testUntil = v.clock().Add(vibratorTestDuration)
	v.signalLocked()
	return nil
}

// Select chooses the vibrator device. The device is claimed for this output,
// and both the previous and the new device are stopped before it is ready.
// The live intensity starts again at zero.
func (v *IntifaceVibrator) Select(ctx context.Context, deviceIndex uint32) error {
	selection, err := v.prepareSelection(deviceIndex)
	if err != nil {
		return err
	}
	v.mu.Lock()
	playing := v.stream.playing
	previous := v.selection
	v.mu.Unlock()
	keepsClaim := previous.selected && previous.deviceIndex == deviceIndex
	if playing {
		v.releaseClaim(deviceIndex, keepsClaim)
		return errVibratorPlaying
	}
	targets := []uint32{deviceIndex}
	if previous.selected && !keepsClaim {
		targets = append(targets, previous.deviceIndex)
	}
	for _, target := range targets {
		stopCtx, cancel := context.WithTimeout(ctx, vibratorStopTimeout)
		err = errors.Join(err, v.session.requestOK(stopCtx, "StopDeviceCmd", map[string]any{"DeviceIndex": target}))
		cancel()
	}
	v.session.mu.Lock()
	if current, ok := v.session.incarnationLocked(deviceIndex); err == nil && (!ok || current != selection.incarnation) {
		err = errors.New("the vibrator disappeared while it was being prepared")
	}
	if err == nil && previous.selected && !keepsClaim {
		v.session.releaseClaimLocked(v, previous.deviceIndex)
	}
	v.session.mu.Unlock()
	if err != nil {
		v.releaseClaim(deviceIndex, keepsClaim)
		return fmt.Errorf("Intiface device %d could not be prepared: %w", deviceIndex, err)
	}
	v.mu.Lock()
	v.selection = selection
	v.epoch++
	v.control++
	v.intensity = 0
	v.stream.state = "idle"
	v.mu.Unlock()
	v.watchedIndex.Store(deviceIndex)
	return nil
}

func (v *IntifaceVibrator) releaseClaim(deviceIndex uint32, keep bool) {
	if keep {
		return
	}
	v.session.mu.Lock()
	v.session.releaseClaimLocked(v, deviceIndex)
	v.session.mu.Unlock()
}

// prepareSelection validates a device and claims it for this output.
func (v *IntifaceVibrator) prepareSelection(deviceIndex uint32) (vibratorSelection, error) {
	s := v.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.connected {
		return vibratorSelection{}, errors.New("Intiface connection is stale")
	}
	device, ok := s.devices[deviceIndex]
	if !ok {
		return vibratorSelection{}, fmt.Errorf("Intiface device %d is not available", deviceIndex)
	}
	gap := time.Duration(device.DeviceMessageTimingGapMillis) * vibratorTimingGapUnit
	if gap > maxIntifaceDeviceTimingGap {
		return vibratorSelection{}, fmt.Errorf("Intiface device %d timing gap %dms exceeds the supported %dms limit", deviceIndex, device.DeviceMessageTimingGapMillis, maxIntifaceDeviceTimingGap.Milliseconds())
	}
	selection := vibratorSelection{selected: true, deviceIndex: deviceIndex, deviceName: device.DeviceName, timingGap: gap}
	selection.incarnation, _ = s.incarnationLocked(deviceIndex)
	for _, actuator := range device.ScalarActuators {
		if actuator.ActuatorType != vibratorActuatorType || actuator.StepCount == 0 {
			continue
		}
		selection.motors = append(selection.motors, vibratorMotor{
			IntifaceVibratorMotor: IntifaceVibratorMotor{Index: actuator.Index, FeatureDescriptor: actuator.FeatureDescriptor, StepCount: int(actuator.StepCount)},
			last:                  -1,
		})
	}
	if len(selection.motors) == 0 {
		return vibratorSelection{}, errVibratorNoMotors
	}
	if err := s.claimLocked(v, deviceIndex); err != nil {
		return vibratorSelection{}, err
	}
	return selection, nil
}

// Release stops and forgets the vibrator while the session stays open for
// other outputs. A Stop that cannot be confirmed fails the session, because
// the server then stops its devices.
func (v *IntifaceVibrator) Release(ctx context.Context) error {
	v.mu.Lock()
	v.invalidateLocked("idle")
	selection := v.selection
	v.selection = vibratorSelection{}
	v.epoch++
	v.intensity = 0
	v.mu.Unlock()
	if !selection.selected {
		return nil
	}
	v.releaseClaim(selection.deviceIndex, false)
	return v.silence(ctx, selection)
}

// SelectionEpoch changes with every selection, release and lost device, so
// an engine built for an older binding is rebuilt.
func (v *IntifaceVibrator) SelectionEpoch() uint64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.epoch
}

// Status returns the vibrator's safe snapshot.
func (v *IntifaceVibrator) Status() IntifaceVibratorStatus {
	v.mu.Lock()
	defer v.mu.Unlock()
	status := IntifaceVibratorStatus{
		Selected:          v.selection.selected,
		DeviceName:        v.selection.deviceName,
		Motors:            []IntifaceVibratorMotor{},
		IntensityPercent:  percent(v.intensity),
		CeilingPercent:    percent(v.config.Ceiling),
		LevelPercent:      percent(v.level),
		PlaybackState:     v.playbackStateLocked(),
		Testing:           !v.testUntil.IsZero(),
		CommandedCount:    v.stats.commanded,
		AcknowledgedCount: v.stats.acked,
		RejectedCount:     v.stats.rejected,
		TimeoutCount:      v.stats.timeouts,
		LastError:         v.stats.lastError,
	}
	if v.selection.selected {
		index := v.selection.deviceIndex
		status.DeviceIndex = &index
		effective := v.config.Ceiling
		for _, motor := range v.selection.motors {
			status.Motors = append(status.Motors, motor.IntifaceVibratorMotor)
			steps := float64(motor.StepCount)
			effective = math.Min(effective, math.Floor(v.config.Ceiling*steps+1e-9)/steps)
		}
		status.EffectiveCeilingPercent = percent(effective)
	}
	return status
}

func (v *IntifaceVibrator) playbackStateLocked() string {
	if v.stalled.Load() {
		return "stale"
	}
	return v.stream.state
}

// invalidateLocked ends playback and any test so no queued level from them
// can reach the wire.
func (v *IntifaceVibrator) invalidateLocked(state string) {
	v.control++
	v.stream.generation++
	v.stream.playing = false
	v.stream.id = ""
	v.stream.timeline.Reset()
	v.stream.origin.Stop()
	v.stream.coverage.Reset()
	v.stream.state = state
	v.testUntil = time.Time{}
	v.level = 0
	v.playingFlag.Store(false)
}

func (v *IntifaceVibrator) signal() {
	v.mu.Lock()
	v.signalLocked()
	v.mu.Unlock()
}

func (v *IntifaceVibrator) signalLocked() {
	select {
	case v.wake <- struct{}{}:
	default:
	}
}

// Session hooks. Locked hooks run with the session lock held; the vibrator
// lock is always taken after it, never before.

func (v *IntifaceVibrator) admitWrite(_ string, _ map[string]any, generation uint64, _ *intifacePacedRequest) (intifaceWriteAdmission, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.stream.playing || v.stream.generation != generation {
		return intifaceWriteAdmission{}, errPacerSuperseded
	}
	return intifaceWriteAdmission{}, nil
}

func (v *IntifaceVibrator) releaseWrite(bool) {}

func (v *IntifaceVibrator) sessionConnectedLocked() {
	v.mu.Lock()
	v.intensity = 0
	v.stream.state = "idle"
	v.diag.Connected = true
	v.mu.Unlock()
}

func (v *IntifaceVibrator) sessionReady(sessionCtx context.Context) {
	v.session.workerMu.Lock()
	defer v.session.workerMu.Unlock()
	if v.session.workersClosed {
		return
	}
	v.lastPace.Store(v.clock().UnixNano())
	if !v.manualPace {
		v.session.wg.Add(1)
		go v.paceLoop(sessionCtx)
	}
	v.session.wg.Add(1)
	go v.watchdog(sessionCtx)
}

func (v *IntifaceVibrator) sessionFailedLocked(err error) func() {
	v.mu.Lock()
	v.invalidateLocked("stale")
	v.selection = vibratorSelection{}
	v.epoch++
	v.intensity = 0
	v.stats.lastError = err.Error()
	v.diag.Connected = false
	v.mu.Unlock()
	return nil
}

func (v *IntifaceVibrator) deviceRemovedLocked(deviceIndex uint32) func() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.selection.selected || v.selection.deviceIndex != deviceIndex {
		return nil
	}
	v.invalidateLocked("stale")
	v.selection = vibratorSelection{}
	v.epoch++
	v.intensity = 0
	return nil
}

// invalidateForEmergency ends playback before StopAllDevices and returns the
// live intensity to zero, as the global Stop does for the Coyote's strength.
func (v *IntifaceVibrator) invalidateForEmergency() {
	v.mu.Lock()
	v.invalidateLocked("idle")
	v.intensity = 0
	v.mu.Unlock()
}

func (v *IntifaceVibrator) invalidateForClose() {
	v.mu.Lock()
	v.invalidateLocked("idle")
	v.mu.Unlock()
}

func (v *IntifaceVibrator) stopForClose(connected bool) error {
	v.mu.Lock()
	selection := v.selection
	v.mu.Unlock()
	if !connected || !selection.selected {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), vibratorStopTimeout)
	defer cancel()
	return v.silenceWithoutFailure(ctx, selection)
}

func (v *IntifaceVibrator) closedLocked() {
	v.mu.Lock()
	v.stream.state = "disconnected"
	v.diag.Connected = false
	v.mu.Unlock()
}

// paceAlive reports false while playback runs on a stalled pacer, so the
// session stops answering pings and the server's own watchdog, when it has
// one, stops the devices too.
func (v *IntifaceVibrator) paceAlive(now time.Time) bool {
	if !v.playingFlag.Load() {
		return true
	}
	return now.Sub(time.Unix(0, v.lastPace.Load())) < vibratorStallLimit
}

func clampUnit(value float64) float64 {
	if math.IsNaN(value) {
		return 0
	}
	return math.Max(0, math.Min(1, value))
}

func percent(fraction float64) int {
	return int(math.Round(clampUnit(fraction) * 100))
}
