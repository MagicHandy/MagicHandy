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
	paused           bool
	pausedPhase      float64
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
	Running          bool                     `json:"running"`
	Paused           bool                     `json:"paused"`
	RunningMillis    int64                    `json:"running_ms"`
	Generation       uint64                   `json:"generation"`
	StreamID         string                   `json:"stream_id,omitempty"`
	PlanID           string                   `json:"plan_id,omitempty"`
	Target           MotionTarget             `json:"target"`
	Settings         config.MotionSettings    `json:"settings"`
	StartedAt        string                   `json:"started_at,omitempty"`
	Phase            float64                  `json:"phase"`
	NextSampleMillis int64                    `json:"next_sample_ms"`
	LastSample       *MotionSample            `json:"last_sample,omitempty"`
	LastResult       *transport.CommandResult `json:"last_result,omitempty"`
	LastError        string                   `json:"last_error,omitempty"`
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
	if err := e.begin(target, settings); err != nil {
		return e.Snapshot(), err
	}
	if err := e.setStrokeWindow(ctx, "start_stroke_window"); err != nil {
		e.forceStopped(err)
		return e.Snapshot(), err
	}
	if err := e.dispatchNextChunk(ctx, "start_points"); err != nil {
		e.forceStopped(err)
		return e.Snapshot(), err
	}
	if err := e.play(ctx); err != nil {
		e.forceStopped(err)
		return e.Snapshot(), err
	}

	e.startLoop(ctx)
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
	if err := e.beginResume(reason); err != nil {
		return e.Snapshot(), err
	}
	if err := e.setStrokeWindow(ctx, "resume_stroke_window"); err != nil {
		e.forceStopped(err)
		e.restorePaused()
		return e.Snapshot(), err
	}
	if err := e.dispatchNextChunk(ctx, "resume_points"); err != nil {
		e.forceStopped(err)
		e.restorePaused()
		return e.Snapshot(), err
	}
	if err := e.play(ctx); err != nil {
		e.forceStopped(err)
		e.restorePaused()
		return e.Snapshot(), err
	}

	e.startLoop(ctx)
	return e.Snapshot(), nil
}

func (e *Engine) beginResume(reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return errors.New("motion is already running")
	}
	if !e.paused {
		return errors.New("motion is not paused")
	}

	e.generation++
	e.streamID = fmt.Sprintf("%s-%06d", e.streamIDPrefix, e.generation)
	e.startedAt = e.now()
	e.nextSampleMillis = 0
	e.lastResult = nil
	e.lastError = ""
	e.latencyMillis = nil
	e.bridgeSample = nil
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
		wasPaused := e.paused
		e.paused = false
		e.runMillisAccum = 0
		e.mu.Unlock()
		if !wasPaused {
			return e.Snapshot(), nil
		}
		// Paused motion already stopped the device; send an explicit stop
		// anyway so the safety command is unconditional.
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

func (e *Engine) begin(target MotionTarget, settings config.MotionSettings) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
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
	e.runMillisAccum = 0
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

func (e *Engine) startLoop(parent context.Context) {
	// Command setup should honor the caller's request context, but the
	// continuous dispatch loop must outlive that request once motion has
	// started. Stop/Pause still cancel the loop through e.cancel.
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(parent))
	done := make(chan struct{})
	e.mu.Lock()
	e.cancel = cancel
	e.done = done
	e.mu.Unlock()

	go func() {
		defer close(done)
		e.runLoop(loopCtx)
	}()
}

func (e *Engine) runLoop(ctx context.Context) {
	ticker := time.NewTicker(e.dispatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.dispatchIfLeadNeeded(ctx, "append_points"); err != nil {
				e.rememberError(err)
			}
		}
	}
}

func (e *Engine) stopLoop() (context.CancelFunc, <-chan struct{}, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return func() {}, nil, false
	}
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

func (e *Engine) stopForRecovery(ctx context.Context, reason string, message string) error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return errors.New(message)
	}
	cancel := e.cancel
	e.running = false
	e.cancel = nil
	e.done = nil
	e.lastError = message
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	result, err := e.transport.Stop(ctx, transport.StopCommand{Reason: reason})
	e.recordTransportResultWithAnnotation(reason, nil, result, err, "recovery="+message)
	e.finishStopped(result, err)
	if err != nil {
		return err
	}
	return errors.New(message)
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
