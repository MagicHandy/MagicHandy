package modes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Mode identifiers.
const (
	// ModeFreestyle is the autonomous arrangement planner.
	ModeFreestyle = "freestyle"
	// ModeChat keeps chat-driven motion alive between turns: it re-applies the
	// last chat target only after transport recovery, never after a user stop
	// or pause.
	ModeChat = "chat"
)

const (
	defaultTickInterval = 250 * time.Millisecond
	restartBackoff      = 3 * time.Second
)

// Engine is the narrow motion-engine surface modes may use. Modes never see
// the transport; the engine owns every dispatch decision.
type Engine interface {
	Start(ctx context.Context, target motion.MotionTarget, settings config.MotionSettings) (motion.ActiveMotionState, error)
	ApplyTarget(ctx context.Context, target motion.MotionTarget, reason string) (motion.ActiveMotionState, error)
	Snapshot() motion.ActiveMotionState
}

// Options wires the manager to the app runtime.
type Options struct {
	// Ensure returns a startable engine for the selected dispatch owner.
	Ensure func(ctx context.Context) (Engine, error)
	// Current returns the live engine or nil when none exists yet.
	Current func() Engine
	// Settings returns the current motion settings snapshot.
	Settings func() config.MotionSettings
	// UsesProceduralGeneration reports whether freestyle should use chaotic HSP.
	UsesProceduralGeneration func() bool
	// ProceduralFreestyleActive reports whether a freestyle chaos segment is playing.
	ProceduralFreestyleActive func() bool
	// StartProceduralFreestyleSegment dispatches one procedural freestyle segment.
	StartProceduralFreestyleSegment func(ctx context.Context, segment ProceduralFreestyleSegment) error
	// StopProceduralFreestyle stops active freestyle procedural playback.
	StopProceduralFreestyle func(ctx context.Context)
	Traces                  *diagnostics.TraceRing
	Now                     func() time.Time
	Tick                    time.Duration
	Seed                    int64
	// MaxSegmentDuration caps armed segment deadlines. It exists for tests
	// that need many segment boundaries quickly; production leaves it zero.
	MaxSegmentDuration time.Duration
}

// Status is the UI-facing mode state.
type Status struct {
	Active         bool   `json:"active"`
	Mode           string `json:"mode,omitempty"`
	Style          string `json:"style,omitempty"`
	SegmentIndex   int    `json:"segment_index,omitempty"`
	SegmentEndsMs  int64  `json:"segment_ends_in_ms,omitempty"`
	LastEvent      string `json:"last_event,omitempty"`
	LastEventAt    string `json:"last_event_at,omitempty"`
	WaitingForChat bool   `json:"waiting_for_chat,omitempty"`
}

// Manager owns at most one active mode loop.
type Manager struct {
	mu sync.Mutex

	options Options

	mode        string
	cancel      context.CancelFunc
	done        chan struct{}
	planner     *Planner
	segment     Segment
	deadline    time.Time
	driftAt     time.Time
	driftDone   bool
	wasPaused   bool
	userStopped bool
	nextRetry   time.Time
	chatTarget  *motion.MotionTarget
	lastEvent   string
	lastEventAt time.Time
	segmentIdx  int
}

// NewManager creates an idle mode manager.
func NewManager(options Options) (*Manager, error) {
	if options.Ensure == nil || options.Current == nil || options.Settings == nil {
		return nil, errors.New("mode manager requires engine and settings accessors")
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.Tick <= 0 {
		options.Tick = defaultTickInterval
	}
	return &Manager{options: options}, nil
}

// Status returns the UI-facing mode state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := Status{
		Active:    m.mode != "",
		Mode:      m.mode,
		LastEvent: m.lastEvent,
	}
	if m.mode != "" {
		status.Style = m.options.Settings().Style
	}
	if !m.lastEventAt.IsZero() {
		status.LastEventAt = m.lastEventAt.UTC().Format(time.RFC3339Nano)
	}
	if m.mode == ModeFreestyle {
		status.SegmentIndex = m.segmentIdx
		if remaining := m.deadline.Sub(m.options.Now()).Milliseconds(); remaining > 0 {
			status.SegmentEndsMs = remaining
		}
	}
	if m.mode == ModeChat {
		status.WaitingForChat = m.chatTarget == nil
	}
	return status
}

// Start activates a mode, replacing any active one.
func (m *Manager) Start(ctx context.Context, mode string) (Status, error) {
	if mode != ModeFreestyle && mode != ModeChat {
		return m.Status(), fmt.Errorf("unknown mode %q", mode)
	}
	m.Stop("mode_switch")

	m.mu.Lock()
	m.mode = mode
	m.userStopped = false
	m.driftDone = true
	m.deadline = time.Time{}
	m.nextRetry = time.Time{}
	if mode == ModeFreestyle {
		m.planner = NewPlanner(m.options.Seed)
		m.segmentIdx = 0
	}
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.cancel = cancel
	done := make(chan struct{})
	m.done = done
	m.mu.Unlock()

	m.trace(mode, "mode_started", nil, "")
	go func() {
		defer close(done)
		m.run(loopCtx, mode)
	}()
	return m.Status(), nil
}

// Stop deactivates the mode loop. It never stops the engine itself — callers
// own that decision (user Stop already stops the engine through its own path).
func (m *Manager) Stop(reason string) {
	if m.options.StopProceduralFreestyle != nil {
		m.options.StopProceduralFreestyle(context.Background())
	}
	m.mu.Lock()
	if m.mode == "" {
		m.mu.Unlock()
		return
	}
	mode := m.mode
	cancel := m.cancel
	done := m.done
	m.mode = ""
	m.cancel = nil
	m.done = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	m.trace(mode, "mode_stopped", nil, reason)
}

// NotifyUserStop records an explicit user stop: the active mode ends and no
// keepalive may restart motion afterwards.
func (m *Manager) NotifyUserStop() {
	m.mu.Lock()
	m.userStopped = true
	m.chatTarget = nil
	m.mu.Unlock()
	m.Stop("user_stop")
}

// NotifyChatTarget adopts a chat-applied target for chat keepalive.
func (m *Manager) NotifyChatTarget(target motion.MotionTarget) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userStopped = false
	copied := target
	m.chatTarget = &copied
}

// NotifyChatStop clears the keepalive target after a chat-driven stop.
func (m *Manager) NotifyChatStop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chatTarget = nil
	m.userStopped = true
}

// Shutdown stops the loop at process exit.
func (m *Manager) Shutdown() {
	m.Stop("shutdown")
}

func (m *Manager) run(ctx context.Context, mode string) {
	ticker := time.NewTicker(m.options.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			switch mode {
			case ModeFreestyle:
				m.tickFreestyle(ctx)
			default:
				m.tickChat(ctx)
			}
		}
	}
}

func (m *Manager) tickFreestyle(ctx context.Context) {
	if m.usesProceduralGeneration() {
		m.tickFreestyleProcedural(ctx)
		return
	}
	engine := m.options.Current()
	var snapshot motion.ActiveMotionState
	if engine != nil {
		snapshot = engine.Snapshot()
	}

	// A user pause suspends planning entirely: the segment clock freezes and
	// nothing restarts motion until the user resumes.
	if snapshot.Paused {
		m.freezeDeadline()
		return
	}
	m.thawDeadline()

	if !snapshot.Running {
		m.mu.Lock()
		stopped := m.userStopped
		retryAt := m.nextRetry
		m.mu.Unlock()
		if stopped {
			// The user stopped motion; freestyle ends rather than fighting it.
			go m.Stop("user_stop_observed")
			return
		}
		if m.options.Now().Before(retryAt) {
			return
		}
		m.startNextSegment(ctx, "freestyle_start")
		return
	}

	now := m.options.Now()
	m.mu.Lock()
	deadline := m.deadline
	driftAt := m.driftAt
	driftDone := m.driftDone
	segment := m.segment
	m.mu.Unlock()

	if !driftDone && now.After(driftAt) {
		if target, ok := segment.DriftTarget("Freestyle", "freestyle"); ok {
			if _, err := engine.ApplyTarget(ctx, target, "freestyle_drift"); err == nil {
				m.trace(ModeFreestyle, "segment_drift", &diagnostics.MotionTracePlanner{
					Mode:              ModeFreestyle,
					Event:             "segment_drift",
					PatternIdentifier: string(segment.PatternID),
					DriftToPercent:    segment.DriftToSpeedPercent,
				}, "")
			}
		}
		m.mu.Lock()
		m.driftDone = true
		m.mu.Unlock()
		return
	}

	if now.After(deadline) {
		m.applyNextSegment(ctx, engine, "freestyle_segment")
	}
}

func (m *Manager) tickFreestyleProcedural(ctx context.Context) {
	if m.options.StartProceduralFreestyleSegment == nil {
		return
	}

	active := m.proceduralFreestyleActive()
	m.mu.Lock()
	stopped := m.userStopped
	retryAt := m.nextRetry
	m.mu.Unlock()

	if !active {
		if stopped {
			go m.Stop("user_stop_observed")
			return
		}
		if m.options.Now().Before(retryAt) {
			return
		}
		m.startNextProceduralSegment(ctx, "freestyle_start")
		return
	}

	now := m.options.Now()
	m.mu.Lock()
	deadline := m.deadline
	m.mu.Unlock()
	if now.After(deadline) {
		m.applyNextProceduralSegment(ctx, "freestyle_segment")
	}
}

func (m *Manager) usesProceduralGeneration() bool {
	if m.options.UsesProceduralGeneration == nil {
		return false
	}
	return m.options.UsesProceduralGeneration()
}

func (m *Manager) proceduralFreestyleActive() bool {
	if m.options.ProceduralFreestyleActive == nil {
		return false
	}
	return m.options.ProceduralFreestyleActive()
}

func (m *Manager) startNextProceduralSegment(ctx context.Context, reason string) {
	engineCtx := context.WithoutCancel(ctx)
	segment := m.nextProceduralSegment()
	if err := m.options.StartProceduralFreestyleSegment(engineCtx, segment); err != nil {
		m.backoff(ModeFreestyle, reason+"_failed", err)
		return
	}
	m.armProceduralSegment(segment)
	m.traceProcedural(reason, segment)
}

func (m *Manager) applyNextProceduralSegment(ctx context.Context, reason string) {
	m.startNextProceduralSegment(ctx, reason)
}

func (m *Manager) nextProceduralSegment() ProceduralFreestyleSegment {
	m.mu.Lock()
	planner := m.planner
	m.mu.Unlock()
	if planner == nil {
		planner = NewPlanner(m.options.Seed)
		m.mu.Lock()
		m.planner = planner
		m.mu.Unlock()
	}
	return NextProceduralFreestyleSegment(planner, m.options.Settings())
}

func (m *Manager) armProceduralSegment(segment ProceduralFreestyleSegment) {
	duration := time.Duration(segment.DurationMillis) * time.Millisecond
	if m.options.MaxSegmentDuration > 0 && duration > m.options.MaxSegmentDuration {
		duration = m.options.MaxSegmentDuration
	}
	now := m.options.Now()
	m.mu.Lock()
	m.segmentIdx++
	m.deadline = now.Add(duration)
	m.nextRetry = time.Time{}
	m.mu.Unlock()
}

func (m *Manager) traceProcedural(reason string, segment ProceduralFreestyleSegment) {
	planner := m.plannerSnapshot()
	row := &diagnostics.MotionTracePlanner{
		Mode:              ModeFreestyle,
		Event:             reason,
		Style:             m.options.Settings().Style,
		SpeedPercent:      segment.Physics.Velocidade,
		DriftToPercent:    segment.Physics.Intensidade,
		DurationMillis:    segment.DurationMillis,
		PatternIdentifier: segment.Physics.TipoBatida,
		Note:              segment.Physics.Regiao,
	}
	if planner != nil {
		row.Seed = planner.Seed()
		row.SegmentIndex = planner.SegmentIndex()
	}
	m.trace(ModeFreestyle, reason, row, "")
}

// startNextSegment starts the engine on a fresh planner segment (first start
// or recovery restart). The engine loop must outlive the mode loop — stopping
// a mode is a planning decision, and the explicit engine stop is a separate,
// deliberate call — so engine starts never inherit the mode's cancellation.
func (m *Manager) startNextSegment(ctx context.Context, reason string) {
	engineCtx := context.WithoutCancel(ctx)
	engine, err := m.options.Ensure(engineCtx)
	if err != nil {
		m.backoff(ModeFreestyle, "start_unavailable", err)
		return
	}
	segment, scores := m.nextPlannedSegment()
	if _, err := engine.Start(engineCtx, segment.Target("Freestyle", "freestyle"), m.options.Settings()); err != nil {
		m.backoff(ModeFreestyle, "start_failed", err)
		return
	}
	m.armSegment(segment)
	m.tracePlanned(reason, segment, scores)
}

// applyNextSegment retargets the running stream to the next planned segment.
// Transitions ride the engine's phase-preserving / low-jump handoff — modes
// never replace streams or touch transport.
func (m *Manager) applyNextSegment(ctx context.Context, engine Engine, reason string) {
	segment, scores := m.nextPlannedSegment()
	if _, err := engine.ApplyTarget(ctx, segment.Target("Freestyle", "freestyle"), reason); err != nil {
		m.backoff(ModeFreestyle, "segment_failed", err)
		return
	}
	m.armSegment(segment)
	m.tracePlanned(reason, segment, scores)
}

func (m *Manager) nextPlannedSegment() (Segment, []diagnostics.PlannerScore) {
	m.mu.Lock()
	planner := m.planner
	m.mu.Unlock()
	if planner == nil {
		planner = NewPlanner(m.options.Seed)
		m.mu.Lock()
		m.planner = planner
		m.mu.Unlock()
	}
	return planner.NextSegment(m.options.Settings())
}

func (m *Manager) armSegment(segment Segment) {
	duration := time.Duration(segment.DurationMillis) * time.Millisecond
	if m.options.MaxSegmentDuration > 0 && duration > m.options.MaxSegmentDuration {
		duration = m.options.MaxSegmentDuration
	}
	now := m.options.Now()
	m.mu.Lock()
	m.segment = segment
	m.segmentIdx++
	m.deadline = now.Add(duration)
	if segment.DriftToSpeedPercent != 0 {
		m.driftAt = now.Add(duration / 2)
		m.driftDone = false
	} else {
		m.driftDone = true
	}
	m.nextRetry = time.Time{}
	m.mu.Unlock()
}

func (m *Manager) tickChat(ctx context.Context) {
	engine := m.options.Current()
	var snapshot motion.ActiveMotionState
	if engine != nil {
		snapshot = engine.Snapshot()
	}
	if snapshot.Running || snapshot.Paused {
		// Paused chat motion stays paused: keepalive never overrides the user.
		return
	}

	m.mu.Lock()
	target := m.chatTarget
	stopped := m.userStopped
	retryAt := m.nextRetry
	m.mu.Unlock()
	if target == nil || stopped {
		return
	}
	if m.options.Now().Before(retryAt) {
		return
	}

	// Motion is idle with a live chat target and no user stop: this is a
	// transport recovery stop, so keep the session moving. As above, the
	// engine loop never inherits the mode loop's cancellation.
	engineCtx := context.WithoutCancel(ctx)
	engineForStart, err := m.options.Ensure(engineCtx)
	if err != nil {
		m.backoff(ModeChat, "keepalive_unavailable", err)
		return
	}
	if _, err := engineForStart.Start(engineCtx, *target, m.options.Settings()); err != nil {
		m.backoff(ModeChat, "keepalive_failed", err)
		return
	}
	m.trace(ModeChat, "chat_keepalive_restart", &diagnostics.MotionTracePlanner{
		Mode:              ModeChat,
		Event:             "chat_keepalive_restart",
		PatternIdentifier: string(target.PatternID),
		SpeedPercent:      target.SpeedPercent,
	}, "")
}

func (m *Manager) freezeDeadline() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.wasPaused {
		m.wasPaused = true
	}
	// Shift the clock forward every paused tick so remaining time is intact.
	m.deadline = m.deadline.Add(m.options.Tick)
	m.driftAt = m.driftAt.Add(m.options.Tick)
}

func (m *Manager) thawDeadline() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wasPaused = false
}

func (m *Manager) backoff(mode string, event string, err error) {
	m.mu.Lock()
	m.nextRetry = m.options.Now().Add(restartBackoff)
	m.mu.Unlock()
	m.trace(mode, event, nil, err.Error())
}

func (m *Manager) tracePlanned(reason string, segment Segment, scores []diagnostics.PlannerScore) {
	planner := m.plannerSnapshot()
	row := &diagnostics.MotionTracePlanner{
		Mode:              ModeFreestyle,
		Event:             reason,
		Style:             m.options.Settings().Style,
		PatternIdentifier: string(segment.PatternID),
		SpeedPercent:      segment.SpeedPercent,
		DriftToPercent:    segment.DriftToSpeedPercent,
		DurationMillis:    segment.DurationMillis,
		Scores:            scores,
	}
	if planner != nil {
		row.Seed = planner.Seed()
		row.SegmentIndex = planner.SegmentIndex()
	}
	m.trace(ModeFreestyle, reason, row, "")
}

func (m *Manager) plannerSnapshot() *Planner {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.planner
}

func (m *Manager) trace(mode string, event string, planner *diagnostics.MotionTracePlanner, note string) {
	m.mu.Lock()
	m.lastEvent = event
	m.lastEventAt = m.options.Now()
	m.mu.Unlock()

	if m.options.Traces == nil {
		return
	}
	if planner == nil {
		planner = &diagnostics.MotionTracePlanner{Mode: mode, Event: event}
	}
	if note != "" {
		planner.Note = note
	}
	m.options.Traces.Add(diagnostics.MotionTraceRow{
		Source:  mode,
		Reason:  event,
		Planner: planner,
	})
}
