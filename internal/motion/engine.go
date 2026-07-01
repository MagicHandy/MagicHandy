package motion

import (
	"context"
	"errors"
	"fmt"
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
	generation       uint64
	streamID         string
	plan             MotionPlan
	settings         config.MotionSettings
	startedAt        time.Time
	nextSampleMillis int64
	lastSample       *MotionSample
	lastResult       *transport.CommandResult
	lastError        string
	cancel           context.CancelFunc
	done             chan struct{}
}

// ActiveMotionState is a safe snapshot of the current motion loop.
type ActiveMotionState struct {
	Running          bool                     `json:"running"`
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

// Stop cancels active motion and sends an explicit transport stop.
func (e *Engine) Stop(ctx context.Context, reason string) (ActiveMotionState, error) {
	if reason == "" {
		reason = "stop"
	}
	cancel, done, active := e.stopLoop()
	if !active {
		return e.Snapshot(), nil
	}
	cancel()
	waitForLoop(done)

	result, err := e.transport.Stop(ctx, transport.StopCommand{Reason: reason})
	e.recordTransportResult(reason, nil, result, err)
	e.finishStopped(result, err)
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

	handoff := e.nextSampleMillis
	e.generation++
	next := e.plan.Retarget(e.planIDLocked(), target, e.settings, handoff, e.now())
	e.plan = next
	e.traceStateLocked(reason, phaseAnnotation(next.PhasePreserved))
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
	handoff := e.nextSampleMillis
	target := NormalizeTarget(e.plan.Target, settings)
	e.generation++
	e.settings = settings
	e.plan = e.plan.Retarget(e.planIDLocked(), target, settings, handoff, e.now())
	e.plan.PhasePreserved = true
	e.traceStateLocked(reason, "phase_preserved=true")
	return true, nil
}

func (e *Engine) startLoop(parent context.Context) {
	loopCtx, cancel := context.WithCancel(parent)
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
			if err := e.dispatchNextChunk(ctx, "append_points"); err != nil {
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

func waitForLoop(done <-chan struct{}) {
	if done != nil {
		<-done
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
