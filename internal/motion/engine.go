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
	defaultStopTimeout      = 15 * time.Second
)

type loopRecoveryContextKey struct{}

var errRunInvalidated = errors.New("motion run was invalidated")

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
	mu        sync.Mutex
	commandMu sync.Mutex

	transport        transport.Transport
	traces           *diagnostics.TraceRing
	now              func() time.Time
	chunkSize        int
	dispatchInterval time.Duration
	sampleInterval   time.Duration
	streamIDPrefix   string

	running                        bool
	starting                       bool
	completing                     bool
	paused                         bool
	pausedPhase                    float64
	frozenPhase                    float64
	pausedTarget                   MotionTarget
	runMillisAccum                 int64
	generation                     uint64
	runEpoch                       uint64
	stopGeneration                 uint64
	stopBarriers                   uint64
	streamID                       string
	plan                           MotionPlan
	settings                       config.MotionSettings
	startedAt                      time.Time
	nextSampleMillis               int64
	lastSample                     *MotionSample
	lastResult                     *transport.CommandResult
	lastError                      string
	latencyMillis                  []int64
	transition                     *planTransition
	preservePlanKnots              bool
	minimumBufferedLeadMillis      int64
	minimumMediaBufferedLeadMillis int64
	positionResolutionPercent      float64
	resolutionAfterStrokeWindow    bool
	maximumChunkPoints             int
	startupWait                    func(context.Context, time.Duration) error
	runCtx                         context.Context
	cancel                         context.CancelFunc
	done                           chan struct{}
}

// ActiveMotionState is a safe snapshot of the current motion loop.
type ActiveMotionState struct {
	Running                    bool                     `json:"running"`
	Starting                   bool                     `json:"starting"`
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
		transport:          options.Transport,
		traces:             options.Traces,
		now:                options.Now,
		chunkSize:          options.ChunkSize,
		dispatchInterval:   options.DispatchInterval,
		sampleInterval:     options.SampleInterval,
		streamIDPrefix:     options.StreamIDPrefix,
		preservePlanKnots:  true,
		maximumChunkPoints: maximumAdaptiveChunkPoints,
		startupWait:        waitForMotionStartup,
	}
	engine.applyDefaults()
	if provider, ok := options.Transport.(transport.MotionTimingCapabilitiesProvider); ok {
		capabilities := provider.MotionTimingCapabilities()
		if capabilities.MinimumPointInterval > engine.sampleInterval {
			engine.sampleInterval = capabilities.MinimumPointInterval
		}
		// An immediate-mode owner with a declared timing floor cannot always
		// represent authored knots that are closer together than that floor.
		// Keep its neutral cadence valid instead of injecting points it must
		// reject. Buffered HSP owners preserve authored knot timing.
		if capabilities.MinimumPointInterval > 0 {
			engine.preservePlanKnots = false
		}
		if capabilities.MinimumBufferedLead > 0 {
			engine.minimumBufferedLeadMillis = max(int64(1), capabilities.MinimumBufferedLead.Milliseconds())
		}
		if capabilities.MinimumMediaBufferedLead > 0 {
			engine.minimumMediaBufferedLeadMillis = max(
				int64(1),
				capabilities.MinimumMediaBufferedLead.Milliseconds(),
			)
		}
	}
	if provider, ok := options.Transport.(transport.MotionSamplingCapabilitiesProvider); ok {
		capabilities := provider.MotionSamplingCapabilities()
		if capabilities.PositionResolutionPercent > 0 && capabilities.PositionResolutionPercent <= 100 {
			engine.positionResolutionPercent = capabilities.PositionResolutionPercent
			engine.resolutionAfterStrokeWindow = capabilities.ResolutionAfterStrokeWindow
		}
		if capabilities.MaximumPointsPerAppend >= 2 && capabilities.MaximumPointsPerAppend < engine.maximumChunkPoints {
			engine.maximumChunkPoints = capabilities.MaximumPointsPerAppend
		}
	}
	return engine, nil
}

// Start begins continuous motion against the configured transport.
func (e *Engine) Start(ctx context.Context, target MotionTarget, settings config.MotionSettings) (ActiveMotionState, error) {
	return e.StartAtGeneration(ctx, target, settings, e.AdmissionGeneration())
}

// AdmissionGeneration snapshots the Stop epoch used to reject work admitted
// before a concurrent Emergency Stop.
func (e *Engine) AdmissionGeneration() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopGeneration
}

// StartAtGeneration starts only when no Stop has occurred since admission.
func (e *Engine) StartAtGeneration(ctx context.Context, target MotionTarget, settings config.MotionSettings, admission uint64) (ActiveMotionState, error) {
	// Create the loop cancel handle up front and publish it atomically with
	// running=true (in begin). The startup transport commands also run on
	// loopCtx, so a concurrent Stop/Pause cancels not just the future loop but
	// the in-flight setup: Stop's cancel() aborts a blocked setStrokeWindow/
	// dispatchNextChunk and makes a not-yet-sent play() fail instead of
	// starting motion the user just stopped. The loop must outlive the request,
	// so loopCtx drops the request's cancellation while keeping its values.
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	runEpoch, err := e.begin(ctx, loopCtx, target, settings, cancel, admission)
	if err != nil {
		cancel()
		return e.Snapshot(), err
	}
	if err := e.prepareMotionStartup(loopCtx, runEpoch, "start"); err != nil {
		e.abortStartup(loopCtx, runEpoch, "start_positioning_failed", err)
		return e.Snapshot(), err
	}
	if err := e.dispatchNextChunk(loopCtx, runEpoch, "start_points"); err != nil {
		e.abortStartup(loopCtx, runEpoch, "start_points_failed", err)
		return e.Snapshot(), err
	}
	if err := e.prebufferBeforePlay(loopCtx, runEpoch, "start_prebuffer_points"); err != nil {
		e.abortStartup(loopCtx, runEpoch, "start_prebuffer_failed", err)
		return e.Snapshot(), err
	}
	if err := e.play(loopCtx, runEpoch); err != nil {
		e.abortStartup(loopCtx, runEpoch, "start_play_failed", err)
		return e.Snapshot(), err
	}
	e.alignPlaybackStart(runEpoch)

	if !e.startLoop(loopCtx, runEpoch) {
		return e.Snapshot(), errRunInvalidated
	}
	return e.Snapshot(), nil
}

// ApplyTarget retargets active motion without stopping the active stream.
func (e *Engine) ApplyTarget(ctx context.Context, target MotionTarget, reason string) (ActiveMotionState, error) {
	if reason == "" {
		reason = "target_applied"
	}
	runEpoch, err := e.activeRunEpoch()
	if err != nil {
		return e.Snapshot(), err
	}
	if err := e.ensureLeadBuffer(ctx, runEpoch, "retarget_lead_points"); err != nil {
		return e.Snapshot(), err
	}
	if err := e.retarget(runEpoch, target, reason); err != nil {
		return e.Snapshot(), err
	}
	err = e.dispatchNextChunk(ctx, runEpoch, "retarget_points")
	return e.Snapshot(), err
}

// RefreshSettings applies active speed, stroke-window, and direction updates.
func (e *Engine) RefreshSettings(ctx context.Context, settings config.MotionSettings, reason string) (ActiveMotionState, error) {
	if reason == "" {
		reason = "settings_refresh"
	}
	runEpoch, active := e.runEpochIfActive()
	if !active {
		e.mu.Lock()
		if !e.running {
			e.settings = normalizeMotionSettings(settings)
			e.mu.Unlock()
			return e.Snapshot(), nil
		}
		e.mu.Unlock()
		return e.Snapshot(), errRunInvalidated
	}
	if err := e.ensureLeadBuffer(ctx, runEpoch, "settings_lead_points"); err != nil {
		return e.Snapshot(), err
	}
	stillActive, err := e.refreshSettings(runEpoch, settings, reason)
	if err != nil {
		return e.Snapshot(), err
	}
	if !stillActive {
		return e.Snapshot(), nil
	}
	if err := e.setStrokeWindow(ctx, runEpoch, "settings_stroke_window", true); err != nil {
		return e.Snapshot(), err
	}
	err = e.dispatchNextChunk(ctx, runEpoch, "settings_points")
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
	pauseAdmission := e.stopGeneration
	e.stopBarriers++
	cancel, done, _ := e.stopLoopLocked()
	e.mu.Unlock()

	cancel()
	waitForLoop(done)

	e.commandMu.Lock()
	stopCtx, stopCancel := detachedStopContext(ctx)
	stopCommand := transport.StopCommand{Reason: reason}
	result, err := e.transport.Stop(stopCtx, stopCommand)
	stopCancel()
	e.recordTransportResult(reason, nil, transport.Command{Kind: transport.CommandKindStop, Stop: &stopCommand}, result, err)
	e.finishStopped(result, err)

	// The loop is dead either way, so the engine is paused even if the
	// transport stop errored; the error stays visible in the snapshot.
	e.mu.Lock()
	if e.stopGeneration == pauseAdmission {
		e.paused = true
		e.traceStateLocked("paused", phaseAnnotation(true))
	}
	e.endStopBarrierLocked()
	e.mu.Unlock()
	e.commandMu.Unlock()
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
	runEpoch, resumeAdmission, err := e.beginResume(loopCtx, reason, cancel)
	if err != nil {
		cancel()
		return e.Snapshot(), err
	}
	if err := e.prepareMotionStartup(loopCtx, runEpoch, "resume"); err != nil {
		e.abortStartup(loopCtx, runEpoch, "resume_positioning_failed", err)
		e.restorePaused(runEpoch, resumeAdmission)
		return e.Snapshot(), err
	}
	if err := e.dispatchNextChunk(loopCtx, runEpoch, "resume_points"); err != nil {
		e.abortStartup(loopCtx, runEpoch, "resume_points_failed", err)
		e.restorePaused(runEpoch, resumeAdmission)
		return e.Snapshot(), err
	}
	if err := e.prebufferBeforePlay(loopCtx, runEpoch, "resume_prebuffer_points"); err != nil {
		e.abortStartup(loopCtx, runEpoch, "resume_prebuffer_failed", err)
		e.restorePaused(runEpoch, resumeAdmission)
		return e.Snapshot(), err
	}
	if err := e.play(loopCtx, runEpoch); err != nil {
		e.abortStartup(loopCtx, runEpoch, "resume_play_failed", err)
		e.restorePaused(runEpoch, resumeAdmission)
		return e.Snapshot(), err
	}
	e.alignPlaybackStart(runEpoch)

	if !e.startLoop(loopCtx, runEpoch) {
		return e.Snapshot(), errRunInvalidated
	}
	return e.Snapshot(), nil
}

func (e *Engine) beginResume(loopCtx context.Context, reason string, cancel context.CancelFunc) (uint64, uint64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running || e.completing || e.stopBarriers > 0 {
		return 0, 0, errors.New("motion is running or stopping")
	}
	if !e.paused {
		return 0, 0, errors.New("motion is not paused")
	}

	e.runEpoch++
	e.runCtx = loopCtx
	e.cancel = cancel
	e.done = nil
	e.generation++
	e.streamID = fmt.Sprintf("%s-%06d", e.streamIDPrefix, e.generation)
	e.startedAt = e.now()
	e.nextSampleMillis = 0
	e.lastResult = nil
	e.lastError = ""
	e.latencyMillis = nil
	e.transition = nil
	e.frozenPhase = e.pausedPhase
	plan := NewMotionPlan(e.planIDLocked(), e.pausedTarget, e.settings, e.pausedPhase, 0, e.startedAt)
	plan.PhasePreserved = true
	e.plan = plan
	e.paused = false
	e.starting = true
	e.running = true
	e.traceStateLocked(reason, phaseAnnotation(true))
	return e.runEpoch, e.stopGeneration, nil
}

// restorePaused re-marks the engine paused after a failed resume so the
// frozen phase and target survive for a retry.
func (e *Engine) restorePaused(runEpoch uint64, stopAdmission uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runEpoch == runEpoch && e.stopGeneration == stopAdmission && !e.running && e.stopBarriers == 0 {
		e.paused = true
	}
}

// Stop cancels active motion and sends an explicit transport stop. It also
// clears any paused state: after Stop there is nothing to resume and the
// run clock resets.
func (e *Engine) Stop(ctx context.Context, reason string) (ActiveMotionState, error) {
	if reason == "" {
		reason = "stop"
	}
	e.mu.Lock()
	e.stopGeneration++
	e.stopBarriers++
	cancel, done, active := e.stopLoopLocked()
	if !active {
		e.paused = false
		e.runMillisAccum = 0
	}
	e.mu.Unlock()
	if active {
		cancel()
		waitForLoop(done)
	}

	// Every Stop activation reaches the transport, including repeated idle
	// and paused stops. commandMu makes this the final mutating wire command
	// for the invalidated run.
	e.commandMu.Lock()
	stopCtx, stopCancel := detachedStopContext(ctx)
	stopCommand := transport.StopCommand{Reason: reason}
	result, err := e.transport.Stop(stopCtx, stopCommand)
	stopCancel()
	e.recordTransportResult(reason, nil, transport.Command{Kind: transport.CommandKindStop, Stop: &stopCommand}, result, err)
	e.finishStopped(result, err)
	e.mu.Lock()
	e.paused = false
	e.runMillisAccum = 0
	e.endStopBarrierLocked()
	e.mu.Unlock()
	e.commandMu.Unlock()
	return e.Snapshot(), err
}

// Snapshot returns the current active motion state.
func (e *Engine) Snapshot() ActiveMotionState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotLocked()
}

func (e *Engine) begin(ctx context.Context, loopCtx context.Context, target MotionTarget, settings config.MotionSettings, cancel context.CancelFunc, admission uint64) (uint64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if e.stopGeneration != admission {
		return 0, errors.New("motion start was invalidated by Stop")
	}
	if e.running || e.completing || e.stopBarriers > 0 {
		return 0, errors.New("motion is running or stopping")
	}

	settings = normalizeMotionSettings(settings)
	e.runEpoch++
	e.generation++
	e.streamID = fmt.Sprintf("%s-%06d", e.streamIDPrefix, e.generation)
	e.settings = settings
	e.startedAt = e.now()
	e.nextSampleMillis = 0
	e.lastSample = nil
	e.lastResult = nil
	e.lastError = ""
	e.latencyMillis = nil
	e.transition = nil
	e.paused = false
	e.completing = false
	e.starting = true
	e.frozenPhase = 0
	e.runMillisAccum = 0
	// The loop cancel is live before running is published, so any concurrent
	// Stop/Pause can cancel it. The done channel is installed later, when the
	// loop goroutine actually launches (startLoop); until then it is nil, and
	// waitForLoop tolerates nil.
	e.cancel = cancel
	e.runCtx = loopCtx
	e.done = nil
	e.plan = NewMotionPlan(e.planIDLocked(), target, settings, 0, 0, e.startedAt)
	e.running = true
	e.traceStateLocked("target_applied", "phase_preserved=false")
	return e.runEpoch, nil
}

func (e *Engine) retarget(runEpoch uint64, target MotionTarget, reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.validateRunLocked(runEpoch); err != nil {
		return err
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
	previousTransition := pruneTransitionHistory(e.transition, estimatedMillis)
	e.transition = previousTransition
	current := sampleMotionPath(previous, previousTransition, estimatedMillis)
	handoffPosition := sampleMotionPath(previous, previousTransition, handoff).PositionPercent
	handoffDirection := motionPathDirection(previous, previousTransition, handoff)
	e.generation++
	next := previous.retargetFromState(
		e.planIDLocked(), target, e.settings, handoff,
		handoffPosition, handoffDirection, now,
	)
	bridgeInserted := transitionRequired(previous, previousTransition, next, handoff)
	if bridgeInserted {
		e.transition = newPlanTransition(previous, previousTransition, handoff)
	} else {
		e.transition = nil
	}
	e.plan = next
	e.traceRetargetLocked(reason, previous, previousSettings, next, e.settings, current, handoff, leadMillis, recentLatency, bridgeInserted, "")
	return nil
}

func (e *Engine) refreshSettings(runEpoch uint64, settings config.MotionSettings, reason string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.validateRunLocked(runEpoch); err != nil {
		return false, err
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
	previousTransition := pruneTransitionHistory(e.transition, estimatedMillis)
	e.transition = previousTransition
	current := sampleMotionPath(previous, previousTransition, estimatedMillis)
	target := NormalizeTarget(e.plan.Target, settings)
	e.generation++
	e.settings = settings
	next := previous.Retarget(e.planIDLocked(), target, settings, handoff, now)
	next.PhasePreserved = true
	bridgeInserted := transitionRequired(previous, previousTransition, next, handoff)
	if bridgeInserted {
		e.transition = newPlanTransition(previous, previousTransition, handoff)
	} else {
		e.transition = nil
	}
	e.plan = next
	e.traceRetargetLocked(reason, previous, previousSettings, next, e.settings, current, handoff, leadMillis, recentLatency, bridgeInserted, "")
	return true, nil
}

// startLoop launches the dispatch goroutine on the context whose cancel was
// installed by begin/beginResume. If a concurrent Stop/Pause flipped running
// false during setup, that stop already cancelled loopCtx and cleared the
// handles, so there is nothing to launch.
func (e *Engine) startLoop(loopCtx context.Context, runEpoch uint64) bool {
	done := make(chan struct{})
	e.mu.Lock()
	if e.validateRunLocked(runEpoch) != nil {
		e.mu.Unlock()
		return false
	}
	e.starting = false
	e.done = done
	e.mu.Unlock()

	go func() {
		defer close(done)
		e.runLoop(loopCtx, runEpoch)
	}()
	return true
}

func (e *Engine) runLoop(ctx context.Context, runEpoch uint64) {
	ctx = context.WithValue(ctx, loopRecoveryContextKey{}, true)
	ticker := time.NewTicker(e.dispatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e.completeProgramIfNeeded(ctx, runEpoch) {
				return
			}
			if err := e.dispatchIfLeadNeeded(ctx, runEpoch, "append_points"); err != nil {
				e.rememberError(err)
			}
		}
	}
}

// completeProgramIfNeeded ends finite content through the engine-owned stop
// path. Repeating patterns never enter this branch.
func (e *Engine) completeProgramIfNeeded(ctx context.Context, runEpoch uint64) bool {
	e.mu.Lock()
	if e.validateRunLocked(runEpoch) != nil || e.plan.Loop || !e.plan.CompleteAt(e.estimatedPlaybackMillisLocked(e.now())) {
		e.mu.Unlock()
		return false
	}
	e.running = false
	e.starting = false
	e.completing = true
	e.stopBarriers++
	cancel := e.cancel
	e.cancel = nil
	e.runCtx = nil
	e.frozenPhase = 1
	e.runMillisAccum = 0
	e.traceStateLocked("program_completed", "finite_content=true")
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	e.commandMu.Lock()
	stopCommand := transport.StopCommand{Reason: "program_completed"}
	stopCtx, stopCancel := detachedStopContext(ctx)
	result, err := e.transport.Stop(stopCtx, stopCommand)
	stopCancel()
	e.recordTransportResult("program_completed", nil, transport.Command{Kind: transport.CommandKindStop, Stop: &stopCommand}, result, err)
	e.finishStopped(result, err)
	e.mu.Lock()
	e.completing = false
	e.cancel = nil
	e.runCtx = nil
	e.done = nil
	e.endStopBarrierLocked()
	e.mu.Unlock()
	e.commandMu.Unlock()
	return true
}

func (e *Engine) stopLoopLocked() (context.CancelFunc, <-chan struct{}, bool) {
	if e.completing {
		return func() {}, e.done, true
	}
	if !e.running {
		return func() {}, nil, false
	}
	e.frozenPhase = e.plan.PhaseAt(e.estimatedPlaybackMillisLocked(e.now()))
	e.running = false
	e.starting = false
	cancel := e.cancel
	done := e.done
	e.cancel = nil
	e.runCtx = nil
	e.done = nil
	if cancel == nil {
		cancel = func() {}
	}
	return cancel, done, true
}

func (e *Engine) endStopBarrierLocked() {
	if e.stopBarriers > 0 {
		e.stopBarriers--
	}
}

func (e *Engine) ensureLeadBuffer(ctx context.Context, runEpoch uint64, reason string) error {
	for range 64 {
		if err := e.ensurePlaybackHealthy(ctx, runEpoch, reason); err != nil {
			return err
		}
		needsLead := false
		var requiredTail int64
		var dispatchTail int64
		e.mu.Lock()
		if e.validateRunLocked(runEpoch) == nil {
			requiredTail = e.estimatedPlaybackMillisLocked(e.now()) + e.leadMillisLocked()
			needsLead = e.bufferedTailMillisLocked() < requiredTail
			if needsLead && e.plan.Target.Media != nil {
				dispatchTail = requiredTail
			}
		} else {
			e.mu.Unlock()
			return errRunInvalidated
		}
		e.mu.Unlock()
		if !needsLead {
			return nil
		}
		if err := e.dispatchThrough(ctx, runEpoch, reason, dispatchTail); err != nil {
			return err
		}
	}
	return errors.New("motion retarget could not build enough lead buffer")
}

func (e *Engine) ensurePlaybackHealthy(ctx context.Context, runEpoch uint64, reason string) error {
	e.mu.Lock()
	if err := e.validateRunLocked(runEpoch); err != nil {
		e.mu.Unlock()
		return err
	}
	starting := e.starting
	e.mu.Unlock()
	diagnostics := e.transport.Diagnostics()
	state := strings.ToLower(strings.TrimSpace(diagnostics.PlaybackState))
	if starting && (state == "not_initialized" || state == "stopped") {
		return nil
	}
	if !playbackStateNeedsRecovery(state) {
		return nil
	}
	message := fmt.Sprintf("motion recovery stopped active stream after playback state %q during %s", diagnostics.PlaybackState, reason)
	return e.stopForRecovery(ctx, runEpoch, "recovery_"+reason, message)
}

// stopForRecovery halts the active stream when playback goes unhealthy. It is
// reachable from the dispatch loop goroutine itself (runLoop ->
// dispatchIfLeadNeeded -> ensurePlaybackHealthy), so loop-originated recovery
// must not wait on the loop's own done channel. Request-originated recovery can
// wait to drain an in-flight append before sending the safety stop.
func (e *Engine) stopForRecovery(ctx context.Context, runEpoch uint64, reason string, message string) error {
	recovery, err := e.prepareRecovery(runEpoch, message)
	if err != nil {
		return err
	}
	return e.finishRecovery(ctx, reason, message, recovery)
}

type recoveryState struct {
	cancel context.CancelFunc
	done   <-chan struct{}
}

func (e *Engine) prepareRecovery(runEpoch uint64, message string) (recoveryState, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.validateRunLocked(runEpoch) != nil {
		return recoveryState{}, errRunInvalidated
	}
	e.stopBarriers++
	recovery := recoveryState{cancel: e.cancel, done: e.done}
	e.frozenPhase = e.plan.PhaseAt(e.estimatedPlaybackMillisLocked(e.now()))
	e.running = false
	e.starting = false
	e.cancel = nil
	e.runCtx = nil
	e.done = nil
	e.lastError = message
	return recovery, nil
}

func (e *Engine) finishRecovery(ctx context.Context, reason string, message string, recovery recoveryState) error {
	if recovery.cancel != nil {
		recovery.cancel()
		if !recoveryFromLoop(ctx) {
			waitForLoop(recovery.done)
		}
	}
	// Cancelling the loop above also cancels ctx when recovery is reached from
	// the dispatch loop. The safety stop must still go out, so send it on a
	// context detached from that cancellation (a cancelled ctx would abort the
	// stop on the real Cloud/Bluetooth transports).
	stopCtx, stopCancel := detachedStopContext(ctx)
	e.commandMu.Lock()
	stopCommand := transport.StopCommand{Reason: reason}
	result, err := e.transport.Stop(stopCtx, stopCommand)
	stopCancel()
	e.recordTransportResultWithAnnotation(reason, nil, transport.Command{
		Kind: transport.CommandKindStop,
		Stop: &stopCommand,
	}, result, err, "recovery="+message)
	e.finishStopped(result, err)
	e.mu.Lock()
	if err == nil {
		e.lastError = message
	}
	e.endStopBarrierLocked()
	e.mu.Unlock()
	e.commandMu.Unlock()
	if err != nil {
		return err
	}
	return errors.New(message)
}

func recoveryFromLoop(ctx context.Context) bool {
	fromLoop, _ := ctx.Value(loopRecoveryContextKey{}).(bool)
	return fromLoop
}

func detachedStopContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), defaultStopTimeout)
	}
	base := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) > 0 {
		return context.WithDeadline(base, deadline)
	}
	return context.WithTimeout(base, defaultStopTimeout)
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

func (e *Engine) alignPlaybackStart(runEpoch uint64) {
	provider, ok := e.transport.(transport.PlaybackStartTimeProvider)
	if !ok {
		return
	}
	startedAt := provider.PlaybackStartTime()
	if startedAt.IsZero() {
		return
	}
	e.mu.Lock()
	if e.validateRunLocked(runEpoch) == nil {
		e.startedAt = startedAt
	}
	e.mu.Unlock()
}

func (e *Engine) leadMillisLocked() int64 {
	// The dispatch loop may notice a low buffer as much as one tick late, and
	// the append still has to cross the owner before the new points are usable.
	// Keep the safety padding as accepted-buffer margin after accounting for
	// both costs rather than spending almost all of it on the polling interval.
	lead := e.recentCommandLatencyMillisLocked() + leadSafetyPaddingMillis + e.dispatchInterval.Milliseconds()
	if lead < minLeadMillis {
		lead = minLeadMillis
	}
	if lead > maxLeadMillis {
		lead = maxLeadMillis
	}
	if minimum := e.selectedMinimumBufferedLeadMillisLocked(); lead < minimum {
		lead = minimum
	}
	return lead
}

func (e *Engine) selectedMinimumBufferedLeadMillisLocked() int64 {
	if e.plan.Target.Media != nil && e.minimumMediaBufferedLeadMillis > 0 {
		return e.minimumMediaBufferedLeadMillis
	}
	return e.minimumBufferedLeadMillis
}

func (e *Engine) bufferedTailMillisLocked() int64 {
	if e.lastSample == nil {
		return 0
	}
	return e.lastSample.TimeMillis
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

func playbackStateNeedsRecovery(state string) bool {
	state = strings.ToLower(strings.TrimSpace(state))
	switch state {
	case "not_initialized", "stopped", "paused", "starved", "starving", "rejected", "stale", "hsp_paused_on_starving", "hsp_starving", "hsp_state_paused", "hsp_state_starving":
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
	} else if e.sampleInterval < time.Millisecond {
		e.sampleInterval = time.Millisecond
	}
	if e.streamIDPrefix == "" {
		e.streamIDPrefix = defaultStreamPrefix
	}
}
