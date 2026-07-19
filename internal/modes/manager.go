package modes

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	// ModeAutopilot is Freestyle's loop with the segment choice delegated to
	// an injected LLM curation step; every failure falls back to the
	// deterministic planner (see autopilot.go).
	ModeAutopilot = "autopilot"
	// ModeChat keeps chat-driven motion alive between turns: it re-applies the
	// last chat target only after transport recovery, never after a user stop
	// or pause.
	ModeChat = "chat"
)

const (
	defaultTickInterval = 250 * time.Millisecond
	restartBackoff      = 3 * time.Second
	modeDwellPadding    = 750 * time.Millisecond
	maximumLatencyDwell = 12 * time.Second
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
	Traces   *diagnostics.TraceRing
	Now      func() time.Time
	Tick     time.Duration
	Seed     int64
	// MaxSegmentDuration caps armed segment deadlines. It exists for tests
	// that need many segment boundaries quickly; production leaves it zero.
	MaxSegmentDuration time.Duration
	// Decide is Autopilot's injected LLM curation step. Autopilot cannot
	// start without it; Freestyle and chat keepalive never use it.
	Decide DecideFunc
	// Announce publishes an Autopilot spoken line to the chat surface after
	// its segment is armed. Optional; failures never affect motion.
	Announce func(say string)
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
	// DecisionSource reports where Autopilot's current segment came from:
	// "model", "fallback" (planner after a failed decision), or "hold".
	DecisionSource string `json:"decision_source,omitempty"`
	// LastSay is the most recent Autopilot spoken line (already in the chat
	// log; surfaced here for the mode card).
	LastSay string `json:"last_say,omitempty"`
}

// Manager owns at most one active mode loop.
type Manager struct {
	lifecycleMu sync.Mutex
	mu          sync.Mutex

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
	generation  uint64
	chatVersion uint64

	recentPatternIDs []string
	decisionSource   string
	lastSay          string

	operationID     uint64
	operationMode   string
	operationCancel context.CancelFunc
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
	mode := m.mode
	lastEvent := m.lastEvent
	lastEventAt := m.lastEventAt
	segmentIdx := m.segmentIdx
	deadline := m.deadline
	waitingForChat := m.chatTarget == nil
	decisionSource := m.decisionSource
	lastSay := m.lastSay
	m.mu.Unlock()

	status := Status{
		Active:    mode != "",
		Mode:      mode,
		LastEvent: lastEvent,
	}
	if mode != "" {
		status.Style = m.options.Settings().Style
	}
	if !lastEventAt.IsZero() {
		status.LastEventAt = lastEventAt.UTC().Format(time.RFC3339Nano)
	}
	if mode == ModeFreestyle || mode == ModeAutopilot {
		status.SegmentIndex = segmentIdx
		if remaining := deadline.Sub(m.options.Now()).Milliseconds(); remaining > 0 {
			status.SegmentEndsMs = remaining
		}
	}
	if mode == ModeAutopilot {
		status.DecisionSource = decisionSource
		status.LastSay = lastSay
	}
	if mode == ModeChat {
		status.WaitingForChat = waitingForChat
	}
	return status
}

// Start activates a mode, replacing any active one.
func (m *Manager) Start(ctx context.Context, mode string) (Status, error) {
	if mode != ModeFreestyle && mode != ModeAutopilot && mode != ModeChat {
		return m.Status(), fmt.Errorf("unknown mode %q", mode)
	}
	if mode == ModeAutopilot && m.options.Decide == nil {
		return m.Status(), errors.New("autopilot requires a configured decision step")
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.stopLoop("mode_switch")

	m.mu.Lock()
	m.generation++
	m.chatVersion++
	m.mode = mode
	m.userStopped = false
	m.chatTarget = nil
	m.driftDone = true
	m.deadline = time.Time{}
	m.nextRetry = time.Time{}
	if mode == ModeFreestyle || mode == ModeAutopilot {
		m.planner = NewPlanner(m.options.Seed)
		m.segmentIdx = 0
		m.segment = Segment{}
		m.recentPatternIDs = nil
		m.decisionSource = ""
		m.lastSay = ""
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
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.stopLoop(reason)
}

func (m *Manager) stopLoop(reason string) {
	m.mu.Lock()
	if m.mode == "" {
		m.mu.Unlock()
		return
	}
	mode := m.mode
	cancel := m.cancel
	done := m.done
	m.generation++
	m.cancelOperationLocked()
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
	finish := m.BeginUserStop()
	finish()
}

// BeginUserStop marks autonomous work unable to restart and cancels its loop
// without waiting. The caller can stop the motion engine first, then invoke the
// returned function to drain and trace the mode goroutine.
func (m *Manager) BeginUserStop() func() {
	m.lifecycleMu.Lock()
	m.mu.Lock()
	m.userStopped = true
	m.chatTarget = nil
	m.chatVersion++
	m.generation++
	m.cancelOperationLocked()
	if m.mode == "" {
		m.mu.Unlock()
		m.lifecycleMu.Unlock()
		return func() {}
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
	var once sync.Once
	return func() {
		once.Do(func() {
			defer m.lifecycleMu.Unlock()
			if done != nil {
				<-done
			}
			m.trace(mode, "mode_stopped", nil, "user_stop")
		})
	}
}

// NotifyChatTarget adopts a chat-applied target for chat keepalive.
func (m *Manager) NotifyChatTarget(target motion.MotionTarget) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userStopped = false
	m.chatVersion++
	if m.operationMode == ModeChat {
		m.cancelOperationLocked()
	}
	copied := cloneTarget(target)
	m.chatTarget = &copied
}

// NotifyChatStop clears the keepalive target after a chat-driven stop.
func (m *Manager) NotifyChatStop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chatTarget = nil
	m.userStopped = true
	m.chatVersion++
	if m.operationMode == ModeChat {
		m.cancelOperationLocked()
	}
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
			if mode == ModeFreestyle || mode == ModeAutopilot {
				m.tickAutonomous(ctx, mode)
			} else {
				m.tickChat(ctx)
			}
		}
	}
}

// tickAutonomous advances Freestyle and Autopilot: both run the same bounded
// segment loop; only the segment source differs (deterministic planner vs
// injected LLM curation with planner fallback — see nextSegmentChoice).
func (m *Manager) tickAutonomous(ctx context.Context, mode string) {
	if ctx.Err() != nil || !m.modeActive(mode) {
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
		generation := m.generation
		m.mu.Unlock()
		if stopped {
			// The user stopped motion; the autonomous mode ends rather than
			// fighting it.
			go m.Stop("user_stop_observed")
			return
		}
		if m.options.Now().Before(retryAt) {
			return
		}
		m.startNextSegment(ctx, mode, mode+"_start", generation)
		return
	}

	now := m.options.Now()
	m.mu.Lock()
	deadline := m.deadline
	driftAt := m.driftAt
	driftDone := m.driftDone
	segment := m.segment
	retryAt := m.nextRetry
	generation := m.generation
	m.mu.Unlock()

	if !driftDone && now.After(driftAt) {
		if target, ok := segment.DriftTarget(modeLabel(mode), mode); ok {
			if _, err := engine.ApplyTarget(ctx, target, mode+"_drift"); err == nil {
				if m.modeGenerationActive(mode, generation) {
					m.trace(mode, "segment_drift", &diagnostics.MotionTracePlanner{
						Mode:              mode,
						Event:             "segment_drift",
						PatternIdentifier: string(segment.PatternID),
						DriftToPercent:    segment.DriftToSpeedPercent,
					}, "")
				}
			}
		}
		m.mu.Lock()
		if m.mode == mode && m.generation == generation {
			m.driftDone = true
		}
		m.mu.Unlock()
		return
	}

	if now.After(deadline) {
		if now.Before(retryAt) {
			return
		}
		m.applyNextSegment(ctx, engine, mode, mode+"_segment", generation)
	}
}

// startNextSegment starts the engine on a fresh segment (first start or
// recovery restart). The engine loop must outlive the mode loop — stopping
// a mode is a planning decision, and the explicit engine stop is a separate,
// deliberate call — so engine starts never inherit the mode's cancellation.
func (m *Manager) startNextSegment(ctx context.Context, mode string, reason string, generation uint64) {
	operationCtx, finish, ok := m.beginStartOperation(ctx, mode, generation, 0)
	if !ok {
		return
	}
	defer finish()

	engine, err := m.options.Ensure(operationCtx)
	if err != nil {
		if operationCtx.Err() != nil {
			return
		}
		m.backoff(mode, generation, "start_unavailable", err)
		return
	}
	choice := m.nextSegmentChoice(operationCtx, mode)
	if operationCtx.Err() != nil {
		return
	}
	state, err := engine.Start(operationCtx, m.choiceTarget(mode, choice), m.options.Settings())
	if err != nil {
		if operationCtx.Err() != nil {
			return
		}
		m.backoff(mode, generation, "start_failed", err)
		return
	}
	m.finishSegmentChoice(mode, reason, choice, state.RecentCommandLatencyMillis, generation)
}

// applyNextSegment retargets the running stream to the next segment.
// Transitions ride the engine's phase-preserving / low-jump handoff — modes
// never replace streams or touch transport.
func (m *Manager) applyNextSegment(ctx context.Context, engine Engine, mode string, reason string, generation uint64) {
	choice := m.nextSegmentChoice(ctx, mode)
	if ctx.Err() != nil || !m.modeGenerationActive(mode, generation) {
		return
	}
	state, err := engine.ApplyTarget(ctx, m.choiceTarget(mode, choice), reason)
	if err != nil {
		m.backoff(mode, generation, "segment_failed", err)
		return
	}
	m.finishSegmentChoice(mode, reason, choice, state.RecentCommandLatencyMillis, generation)
}

// choiceTarget builds the engine target for one segment choice, attaching any
// resolved library pattern definition from an Autopilot curation decision.
func (m *Manager) choiceTarget(mode string, choice segmentChoice) motion.MotionTarget {
	target := choice.segment.Target(modeLabel(mode), mode)
	if choice.pattern != nil {
		target.Pattern = choice.pattern
	}
	return target
}

// finishSegmentChoice arms the segment, records provenance, traces the
// decision, and publishes any Autopilot spoken line — strictly after the
// segment is armed, so a raced stop or mode switch never speaks.
func (m *Manager) finishSegmentChoice(mode string, reason string, choice segmentChoice, recentLatencyMillis int64, generation uint64) {
	if !m.armSegment(mode, choice.segment, recentLatencyMillis, generation) {
		return
	}
	m.rememberChoice(mode, choice)
	m.tracePlanned(mode, reason, choice)
	if mode == ModeAutopilot && choice.say != "" && m.options.Announce != nil {
		m.options.Announce(choice.say)
	}
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

func (m *Manager) armSegment(mode string, segment Segment, recentLatencyMillis int64, generation uint64) bool {
	duration := time.Duration(segment.DurationMillis) * time.Millisecond
	latencyFloor := time.Duration(max(int64(0), recentLatencyMillis))*time.Millisecond + modeDwellPadding
	if latencyFloor > maximumLatencyDwell {
		latencyFloor = maximumLatencyDwell
	}
	if duration < latencyFloor {
		duration = latencyFloor
	}
	if m.options.MaxSegmentDuration > 0 && duration > m.options.MaxSegmentDuration {
		duration = m.options.MaxSegmentDuration
	}
	now := m.options.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mode != mode || m.generation != generation || m.userStopped {
		return false
	}
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
	return true
}

func (m *Manager) tickChat(ctx context.Context) {
	if ctx.Err() != nil || !m.modeActive(ModeChat) {
		return
	}
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
	var target *motion.MotionTarget
	if m.chatTarget != nil {
		copied := cloneTarget(*m.chatTarget)
		target = &copied
	}
	stopped := m.userStopped
	retryAt := m.nextRetry
	generation := m.generation
	chatVersion := m.chatVersion
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
	operationCtx, finish, ok := m.beginStartOperation(ctx, ModeChat, generation, chatVersion)
	if !ok {
		return
	}
	defer finish()

	engineForStart, err := m.options.Ensure(operationCtx)
	if err != nil {
		if operationCtx.Err() != nil {
			return
		}
		m.backoff(ModeChat, generation, "keepalive_unavailable", err)
		return
	}
	if _, err := engineForStart.Start(operationCtx, *target, m.options.Settings()); err != nil {
		if operationCtx.Err() != nil {
			return
		}
		m.backoff(ModeChat, generation, "keepalive_failed", err)
		return
	}
	if !m.chatOperationActive(generation, chatVersion) {
		return
	}
	m.trace(ModeChat, "chat_keepalive_restart", &diagnostics.MotionTracePlanner{
		Mode:              ModeChat,
		Event:             "chat_keepalive_restart",
		PatternIdentifier: string(target.PatternID),
		SpeedPercent:      target.SpeedPercent,
	}, "")
}

func (m *Manager) modeActive(mode string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode == mode && !m.userStopped
}

func (m *Manager) modeGenerationActive(mode string, generation uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode == mode && m.generation == generation && !m.userStopped
}

func (m *Manager) chatOperationActive(generation, chatVersion uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode == ModeChat && m.generation == generation && m.chatVersion == chatVersion &&
		m.chatTarget != nil && !m.userStopped
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

func (m *Manager) backoff(mode string, generation uint64, event string, err error) {
	m.mu.Lock()
	if m.mode != mode || m.generation != generation || m.userStopped {
		m.mu.Unlock()
		return
	}
	m.nextRetry = m.options.Now().Add(restartBackoff)
	m.mu.Unlock()
	m.trace(mode, event, nil, err.Error())
}

func (m *Manager) beginStartOperation(
	parent context.Context,
	mode string,
	generation uint64,
	chatVersion uint64,
) (context.Context, func(), bool) {
	m.mu.Lock()
	if m.mode != mode || m.generation != generation || m.userStopped ||
		(mode == ModeChat && (m.chatVersion != chatVersion || m.chatTarget == nil)) {
		m.mu.Unlock()
		return nil, nil, false
	}
	operationCtx, cancel := context.WithCancel(parent)
	m.operationID++
	id := m.operationID
	m.operationMode = mode
	m.operationCancel = cancel
	m.mu.Unlock()

	return operationCtx, func() {
		cancel()
		m.mu.Lock()
		if m.operationID == id {
			m.operationMode = ""
			m.operationCancel = nil
		}
		m.mu.Unlock()
	}, true
}

func (m *Manager) cancelOperationLocked() {
	if m.operationCancel != nil {
		m.operationCancel()
	}
}

func cloneTarget(target motion.MotionTarget) motion.MotionTarget {
	if target.AreaFocus != nil {
		focus := *target.AreaFocus
		target.AreaFocus = &focus
	}
	if target.SoftAnchor != nil {
		anchor := *target.SoftAnchor
		target.SoftAnchor = &anchor
	}
	if target.Pattern != nil {
		pattern := *target.Pattern
		pattern.Points = append([]motion.CurvePoint(nil), pattern.Points...)
		pattern.Tags = append([]string(nil), pattern.Tags...)
		target.Pattern = &pattern
	}
	if target.Program != nil {
		program := *target.Program
		program.Points = append([]motion.CurvePoint(nil), program.Points...)
		target.Program = &program
	}
	return target
}

func (m *Manager) tracePlanned(mode string, reason string, choice segmentChoice) {
	planner := m.plannerSnapshot()
	row := &diagnostics.MotionTracePlanner{
		Mode:              mode,
		Event:             reason,
		Style:             m.options.Settings().Style,
		PatternIdentifier: string(choice.segment.PatternID),
		SpeedPercent:      choice.segment.SpeedPercent,
		DriftToPercent:    choice.segment.DriftToSpeedPercent,
		DurationMillis:    choice.segment.DurationMillis,
		Scores:            choice.scores,
	}
	if planner != nil {
		row.Seed = planner.Seed()
		row.SegmentIndex = planner.SegmentIndex()
	}
	note := choice.note
	if mode == ModeAutopilot {
		// The decision source and say-presence stay visible in the trace so
		// "why did it do that?" always has an answer.
		note = strings.TrimSpace(choice.source + " " + note)
		if choice.say != "" {
			note += " say"
		}
	}
	m.trace(mode, reason, row, note)
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
