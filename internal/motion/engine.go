package motion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const (
	defaultChunkSize        = 8
	defaultDispatchInterval = 200 * time.Millisecond
	defaultSampleInterval   = 125 * time.Millisecond
	defaultStreamPrefix     = "motion"

	latencySampleLimit      = 8
	leadSafetyPaddingMillis = int64(250)
	minLeadMillis           = int64(500)
	maxLeadMillis           = int64(2000)
)

type loopRecoveryContextKey struct{}

// EngineOptions configures a motion engine instance.
type EngineOptions struct {
	Transport        transport.Transport
	Traces           *diagnostics.TraceRing
	Now              func() time.Time
	ChunkSize        int
	DispatchInterval time.Duration
	SampleInterval   time.Duration
	StreamIDPrefix   string
}

// Engine owns one active semantic motion loop.
type Engine struct {
	mu sync.Mutex

	transport        transport.Transport
	traces           *diagnostics.TraceRing
	now              func() time.Time
	chunkSize        int
	dispatchInterval time.Duration
	sampleInterval   time.Duration
	streamIDPrefix   string

	running          bool
	completing       bool
	paused           bool
	pausedPhase      float64
	frozenPhase      float64
	pausedTarget     MotionTarget
	runMillisAccum   int64
	generation       uint64
	streamID         string
	plan             MotionPlan
	settings         config.MotionSettings
	startedAt        time.Time
	nextSampleMillis int64
	lastSample       *MotionSample
	lastResult       *transport.CommandResult
	lastError        string
	latencyMillis    []int64
	bridgeSample     *MotionSample
	cancel           context.CancelFunc
	done             chan struct{}
}

// ActiveMotionState is a safe snapshot of the current motion loop.
type ActiveMotionState struct {
	Running                    bool                     `json:"running"`
	Completing                 bool                     `json:"completing"`
	Paused                     bool                     `json:"paused"`
	RunningMillis              int64                    `json:"running_ms"`
	Generation                 uint64                   `json:"generation"`
	StreamID                   string                   `json:"stream_id,omitempty"`
	PlanID                     string                   `json:"plan_id,omitempty"`
	Target                     MotionTarget             `json:"target"`
	Settings                   config.MotionSettings    `json:"settings"`
	StartedAt                  string                   `json:"started_at,omitempty"`
	Phase                      float64                  `json:"phase"`
	NextSampleMillis           int64                    `json:"next_sample_ms"`
	RecentCommandLatencyMillis int64                    `json:"recent_command_latency_ms"`
	LastSample                 *MotionSample            `json:"last_sample,omitempty"`
	LastResult                 *transport.CommandResult `json:"last_result,omitempty"`
	LastError                  string                   `json:"last_error,omitempty"`
}

// NewEngine creates a motion engine bound to one transport dispatcher.
func NewEngine(options EngineOptions) (*Engine, error) {
	if options.Transport == nil {
		return nil, errors.New("motion transport is required")
	}
	engine := &Engine{
		transport:        options.Transport,
		traces:           options.Traces,
		now:              options.Now,
		chunkSize:        options.ChunkSize,
		dispatchInterval: options.DispatchInterval,
		sampleInterval:   options.SampleInterval,
		streamIDPrefix:   options.StreamIDPrefix,
	}
	engine.applyDefaults()
	return engine, nil
}

// Start begins continuous motion against the configured transport.
func (e *Engine) Start(ctx context.Context, target MotionTarget, settings config.MotionSettings) (ActiveMotionState, error) {
	// Create the loop cancel handle up front and publish it atomically with
	// running=true (in begin). The startup transport commands also run on
	// loopCtx, so a concurrent Stop/Pause cancels not just the future loop but
	// the in-flight setup: Stop's cancel() aborts a blocked setStrokeWindow/
	// dispatchNextChunk and makes a not-yet-sent play() fail instead of
	// starting motion the user just stopped. The loop must outlive the request,
	// so loopCtx drops the request's cancellation while keeping its values.
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	if err := e.begin(target, settings, cancel); err != nil {
		cancel()
		return e.Snapshot(), err
	}
	if err := e.setStrokeWindow(loopCtx, "start_stroke_window"); err != nil {
		e.forceStopped(err)
		return e.Snapshot(), err
	}
	if err := e.dispatchNextChunk(loopCtx, "start_points"); err != nil {
		e.forceStopped(err)
		return e.Snapshot(), err
	}
	if err := e.play(loopCtx); err != nil {
		e.forceStopped(err)
		return e.Snapshot(), err
	}

	e.startLoop(loopCtx)
	return e.Snapshot(), nil
}

// ApplyTarget retargets active motion without stopping the active stream.
func (e *Engine) ApplyTarget(ctx context.Context, target MotionTarget, reason string) (ActiveMotionState, error) {
	if reason == "" {
		reason = "target_applied"
	}
	if err := e.ensureLeadBuffer(ctx, "retarget_lead_points"); err != nil {
		return e.Snapshot(), err
	}
	if err := e.retarget(target, reason); err != nil {
		return e.Snapshot(), err
	}
	err := e.dispatchNextChunk(ctx, "retarget_points")
	return e.Snapshot(), err
}

// RefreshSettings applies active speed, stroke-window, and direction updates.
func (e *Engine) RefreshSettings(ctx context.Context, settings config.MotionSettings, reason string) (ActiveMotionState, error) {
	if reason == "" {
		reason = "settings_refresh"
	}
	if err := e.ensureLeadBuffer(ctx, "settings_lead_points"); err != nil {
		return e.Snapshot(), err
	}
	active, err := e.refreshSettings(settings, reason)
	if err != nil {
		return e.Snapshot(), err
	}
	if !active {
		return e.Snapshot(), nil
	}
	if err := e.setStrokeWindow(ctx, "settings_stroke_window"); err != nil {
		return e.Snapshot(), err
	}
	err = e.dispatchNextChunk(ctx, "settings_points")
	return e.Snapshot(), err
}

// Pause stops dispatch and the device while freezing the plan phase, so
// Resume can continue the same pattern where it left off. It is not the
// safety path: Stop clears everything, Pause deliberately retains state.
func (e *Engine) Pause(ctx context.Context, reason string) (ActiveMotionState, error) {
	if reason == "" {
		reason = "pause"
	}
	e.mu.Lock()
	if e.paused && !e.running {
		e.mu.Unlock()
		return e.Snapshot(), nil
	}
	if !e.running {
		e.mu.Unlock()
		return e.Snapshot(), errors.New("motion is not running")
	}
	played := e.estimatedPlaybackMillisLocked(e.now())
	e.pausedPhase = e.plan.PhaseAt(played)
	e.frozenPhase = e.pausedPhase
	e.pausedTarget = e.plan.Target
	e.runMillisAccum += played
	e.mu.Unlock()

	cancel, done, active := e.stopLoop()
	if active {
		cancel()
		waitForLoop(done)
	}

	result, err := e.transport.Stop(ctx, transport.StopCommand{Reason: reason})
	e.recordTransportResult(reason, nil, result, err)
	e.finishStopped(result, err)

	// The loop is dead either way, so the engine is paused even if the
	// transport stop errored; the error stays visible in the snapshot.
	e.mu.Lock()
	e.paused = true
	e.traceStateLocked("paused", phaseAnnotation(true))
	e.mu.Unlock()
	return e.Snapshot(), err
}

// Resume continues paused motion with the same target and preserved phase on
// a fresh stream.
func (e *Engine) Resume(ctx context.Context, reason string) (ActiveMotionState, error) {
	if reason == "" {
		reason = "resume"
	}
	// Startup commands run on loopCtx so a concurrent Stop cancels in-flight
	// resume setup, same as Start.
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	if err := e.beginResume(reason, cancel); err != nil {
		cancel()
		return e.Snapshot(), err
	}
	if err := e.setStrokeWindow(loopCtx, "resume_stroke_window"); err != nil {
		e.forceStopped(err)
		e.restorePaused()
		return e.Snapshot(), err
	}
	if err := e.dispatchNextChunk(loopCtx, "resume_points"); err != nil {
		e.forceStopped(err)
		e.restorePaused()
		return e.Snapshot(), err
	}
	if err := e.play(loopCtx); err != nil {
		e.forceStopped(err)
		e.restorePaused()
		return e.Snapshot(), err
	}

	e.startLoop(loopCtx)
	return e.Snapshot(), nil
}

func (e *Engine) beginResume(reason string, cancel context.CancelFunc) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running || e.completing {
		return errors.New("motion is already running")
	}
	if !e.paused {
		return errors.New("motion is not paused")
	}

	e.cancel = cancel
	e.done = nil
	e.generation++
	e.streamID = fmt.Sprintf("%s-%06d", e.streamIDPrefix, e.generation)
	e.startedAt = e.now()
	e.nextSampleMillis = 0
	e.lastResult = nil
	e.lastError = ""
	e.latencyMillis = nil
	e.bridgeSample = nil
	e.frozenPhase = e.pausedPhase
	plan := NewMotionPlan(e.planIDLocked(), e.pausedTarget, e.settings, e.pausedPhase, 0, e.startedAt)
	plan.PhasePreserved = true
	e.plan = plan
	e.paused = false
	e.running = true
	e.traceStateLocked(reason, phaseAnnotation(true))
	return nil
}

// restorePaused re-marks the engine paused after a failed resume so the
// frozen phase and target survive for a retry.
func (e *Engine) restorePaused() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.paused = true
}

// Stop cancels active motion and sends an explicit transport stop. It also
// clears any paused state: after Stop there is nothing to resume and the
// run clock resets.
func (e *Engine) Stop(ctx context.Context, reason string) (ActiveMotionState, error) {
	if reason == "" {
		reason = "stop"
	}
	cancel, done, active := e.stopLoop()
	if !active {
		e.mu.Lock()
		e.paused = false
		e.runMillisAccum = 0
		e.mu.Unlock()
		// Every Stop activation reaches the transport, including repeated
		// idle and paused stops. Device state can outlive engine state.
		result, err := e.transport.Stop(ctx, transport.StopCommand{Reason: reason})
		e.recordTransportResult(reason, nil, result, err)
		e.finishStopped(result, err)
		return e.Snapshot(), err
	}
	cancel()
	waitForLoop(done)

	result, err := e.transport.Stop(ctx, transport.StopCommand{Reason: reason})
	e.recordTransportResult(reason, nil, result, err)
	e.finishStopped(result, err)
	e.mu.Lock()
	e.paused = false
	e.runMillisAccum = 0
	e.mu.Unlock()
	return e.Snapshot(), err
}

// Snapshot returns the current active motion state.
func (e *Engine) Snapshot() ActiveMotionState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotLocked()
}

func (e *Engine) begin(target MotionTarget, settings config.MotionSettings, cancel context.CancelFunc) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running || e.completing {
		return errors.New("motion is already running")
	}

	settings = normalizeMotionSettings(settings)
	e.generation++
	e.streamID = fmt.Sprintf("%s-%06d", e.streamIDPrefix, e.generation)
	e.settings = settings
	e.startedAt = e.now()
	e.nextSampleMillis = 0
	e.lastSample = nil
	e.lastResult = nil
	e.lastError = ""
	e.latencyMillis = nil
	e.bridgeSample = nil
	e.paused = false
	e.completing = false
	e.frozenPhase = 0
	e.runMillisAccum = 0
	// The loop cancel is live before running is published, so any concurrent
	// Stop/Pause can cancel it. The done channel is installed later, when the
	// loop goroutine actually launches (startLoop); until then it is nil, and
	// waitForLoop tolerates nil.
	e.cancel = cancel
	e.done = nil
	e.plan = NewMotionPlan(e.planIDLocked(), target, settings, 0, 0, e.startedAt)
	e.running = true
	e.traceStateLocked("target_applied", "phase_preserved=false")
	return nil
}

func (e *Engine) retarget(target MotionTarget, reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return errors.New("motion is not running")
	}

	now := e.now()
	handoff := e.nextSampleMillis
	estimatedMillis := e.estimatedPlaybackMillisLocked(now)
	leadMillis := handoff - estimatedMillis
	if leadMillis < 0 {
		leadMillis = 0
	}
	recentLatency := e.recentCommandLatencyMillisLocked()
	previous := e.plan
	previousSettings := e.settings
	current := previous.SampleAt(estimatedMillis)
	previousHandoff := previous.SampleAt(handoff)
	e.generation++
	next := previous.Retarget(e.planIDLocked(), target, e.settings, handoff, now)
	nextHandoff := next.SampleAt(handoff)
	bridgeInserted := shouldInsertBridgePoint(previousHandoff, nextHandoff)
	if bridgeInserted {
		bridge := previousHandoff
		bridge.PlanID = next.ID
		e.bridgeSample = &bridge
	} else {
		e.bridgeSample = nil
	}
	e.plan = next
	e.traceRetargetLocked(reason, previous, previousSettings, next, e.settings, current, handoff, leadMillis, recentLatency, bridgeInserted, "")
	return nil
}

func (e *Engine) refreshSettings(settings config.MotionSettings, reason string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		e.settings = normalizeMotionSettings(settings)
		return false, nil
	}

	settings = normalizeMotionSettings(settings)
	now := e.now()
	handoff := e.nextSampleMillis
	estimatedMillis := e.estimatedPlaybackMillisLocked(now)
	leadMillis := handoff - estimatedMillis
	if leadMillis < 0 {
		leadMillis = 0
	}
	recentLatency := e.recentCommandLatencyMillisLocked()
	previous := e.plan
	previousSettings := e.settings
	current := previous.SampleAt(estimatedMillis)
	target := NormalizeTarget(e.plan.Target, settings)
	e.generation++
	e.settings = settings
	next := previous.Retarget(e.planIDLocked(), target, settings, handoff, now)
	next.PhasePreserved = true
	previousHandoff := previous.SampleAt(handoff)
	nextHandoff := next.SampleAt(handoff)
	bridgeInserted := shouldInsertBridgePoint(previousHandoff, nextHandoff)
	if bridgeInserted {
		bridge := previousHandoff
		bridge.PlanID = next.ID
		e.bridgeSample = &bridge
	} else {
		e.bridgeSample = nil
	}
	e.plan = next
	e.traceRetargetLocked(reason, previous, previousSettings, next, e.settings, current, handoff, leadMillis, recentLatency, bridgeInserted, "")
	return true, nil
}

// startLoop launches the dispatch goroutine on the context whose cancel was
// installed by begin/beginResume. If a concurrent Stop/Pause flipped running
// false during setup, that stop already cancelled loopCtx and cleared the
// handles, so there is nothing to launch.
func (e *Engine) startLoop(loopCtx context.Context) {
	done := make(chan struct{})
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.done = done
	e.mu.Unlock()

	go func() {
		defer close(done)
		e.runLoop(loopCtx)
	}()
}

func (e *Engine) runLoop(ctx context.Context) {
	ctx = context.WithValue(ctx, loopRecoveryContextKey{}, true)
	ticker := time.NewTicker(e.dispatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e.completeProgramIfNeeded(ctx) {
				return
			}
			if err := e.dispatchIfLeadNeeded(ctx, "append_points"); err != nil {
				e.rememberError(err)
			}
		}
	}
}

// completeProgramIfNeeded ends finite content through the engine-owned stop
// path. Repeating patterns never enter this branch.
func (e *Engine) completeProgramIfNeeded(ctx context.Context) bool {
	e.mu.Lock()
	if !e.running || e.plan.Loop || !e.plan.CompleteAt(e.estimatedPlaybackMillisLocked(e.now())) {
		e.mu.Unlock()
		return false
	}
	e.running = false
	e.completing = true
	e.frozenPhase = 1
	e.runMillisAccum = 0
	e.traceStateLocked("program_completed", "finite_content=true")
	e.mu.Unlock()

	result, err := e.transport.Stop(context.WithoutCancel(ctx), transport.StopCommand{Reason: "program_completed"})
	e.recordTransportResult("program_completed", nil, result, err)
	e.finishStopped(result, err)
	e.mu.Lock()
	e.completing = false
	e.cancel = nil
	e.done = nil
	e.mu.Unlock()
	return true
}

func (e *Engine) stopLoop() (context.CancelFunc, <-chan struct{}, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.completing {
		return func() {}, e.done, true
	}
	if !e.running {
		return func() {}, nil, false
	}
	e.frozenPhase = e.plan.PhaseAt(e.estimatedPlaybackMillisLocked(e.now()))
	e.running = false
	cancel := e.cancel
	done := e.done
	e.cancel = nil
	e.done = nil
	return cancel, done, true
}

func (e *Engine) ensureLeadBuffer(ctx context.Context, reason string) error {
	for range 64 {
		if err := e.ensurePlaybackHealthy(ctx, reason); err != nil {
			return err
		}
		needsLead := false
		e.mu.Lock()
		if e.running {
			requiredTail := e.estimatedPlaybackMillisLocked(e.now()) + e.leadMillisLocked()
			needsLead = e.nextSampleMillis < requiredTail
		}
		e.mu.Unlock()
		if !needsLead {
			return nil
		}
		if err := e.dispatchNextChunk(ctx, reason); err != nil {
			return err
		}
	}
	return errors.New("motion retarget could not build enough lead buffer")
}

func (e *Engine) ensurePlaybackHealthy(ctx context.Context, reason string) error {
	diagnostics := e.transport.Diagnostics()
	if !playbackStateNeedsRecovery(diagnostics.PlaybackState) {
		return nil
	}
	message := fmt.Sprintf("motion recovery stopped active stream after playback state %q during %s", diagnostics.PlaybackState, reason)
	return e.stopForRecovery(ctx, "recovery_"+reason, message)
}

// stopForRecovery halts the active stream when playback goes unhealthy. It is
// reachable from the dispatch loop goroutine itself (runLoop ->
// dispatchIfLeadNeeded -> ensurePlaybackHealthy), so loop-originated recovery
// must not wait on the loop's own done channel. Request-originated recovery can
// wait to drain an in-flight append before sending the safety stop.
func (e *Engine) stopForRecovery(ctx context.Context, reason string, message string) error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return errors.New(message)
	}
	cancel := e.cancel
	done := e.done
	e.frozenPhase = e.plan.PhaseAt(e.estimatedPlaybackMillisLocked(e.now()))
	e.running = false
	e.cancel = nil
	e.done = nil
	e.lastError = message
	e.mu.Unlock()

	if cancel != nil {
		cancel()
		if !recoveryFromLoop(ctx) {
			waitForLoop(done)
		}
	}
	// Cancelling the loop above also cancels ctx when recovery is reached from
	// the dispatch loop. The safety stop must still go out, so send it on a
	// context detached from that cancellation (a cancelled ctx would abort the
	// stop on the real Cloud/Bluetooth transports).
	stopCtx := context.WithoutCancel(ctx)
	result, err := e.transport.Stop(stopCtx, transport.StopCommand{Reason: reason})
	e.recordTransportResultWithAnnotation(reason, nil, result, err, "recovery="+message)
	e.finishStopped(result, err)
	if err != nil {
		return err
	}
	return errors.New(message)
}

func recoveryFromLoop(ctx context.Context) bool {
	fromLoop, _ := ctx.Value(loopRecoveryContextKey{}).(bool)
	return fromLoop
}

func (e *Engine) estimatedPlaybackMillisLocked(now time.Time) int64 {
	if e.startedAt.IsZero() {
		return 0
	}
	elapsed := now.Sub(e.startedAt).Milliseconds()
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func (e *Engine) leadMillisLocked() int64 {
	lead := e.recentCommandLatencyMillisLocked() + leadSafetyPaddingMillis
	if lead < minLeadMillis {
		return minLeadMillis
	}
	if lead > maxLeadMillis {
		return maxLeadMillis
	}
	return lead
}

func (e *Engine) recentCommandLatencyMillisLocked() int64 {
	var recent int64
	for _, latency := range e.latencyMillis {
		if latency > recent {
			recent = latency
		}
	}
	return recent
}

func waitForLoop(done <-chan struct{}) {
	if done != nil {
		<-done
	}
}

func shouldInsertBridgePoint(previous MotionSample, next MotionSample) bool {
	const maxRetargetJumpPercent = 12
	delta := previous.PositionPercent - next.PositionPercent
	if delta < 0 {
		delta = -delta
	}
	return delta > maxRetargetJumpPercent
}

func playbackStateNeedsRecovery(state string) bool {
	state = strings.ToLower(strings.TrimSpace(state))
	switch state {
	case "paused", "starved", "rejected", "stale", "hsp_paused_on_starving", "hsp_starving", "hsp_state_paused", "hsp_state_starving":
		return true
	default:
		return false
	}
}

func (e *Engine) applyDefaults() {
	if e.now == nil {
		e.now = func() time.Time { return time.Now().UTC() }
	}
	if e.chunkSize <= 0 {
		e.chunkSize = defaultChunkSize
	}
	if e.dispatchInterval <= 0 {
		e.dispatchInterval = defaultDispatchInterval
	}
	if e.sampleInterval <= 0 {
		e.sampleInterval = defaultSampleInterval
	}
	if e.streamIDPrefix == "" {
		e.streamIDPrefix = defaultStreamPrefix
	}
}
