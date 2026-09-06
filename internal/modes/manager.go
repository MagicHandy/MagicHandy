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
	// AutopilotSettings returns durable cadence and speech-authority settings.
	AutopilotSettings func() config.AutopilotSettings
	// MotionGenerationMode keeps Autopilot fallback inside the selected model
	// vocabulary. Dynamic mode never falls through to the pattern planner.
	MotionGenerationMode func() string
	Traces               *diagnostics.TraceRing
	Now                  func() time.Time
	Tick                 time.Duration
	Seed                 int64
	// MaxSegmentDuration caps armed segment deadlines. It exists for tests
	// that need many segment boundaries quickly; production leaves it zero.
	MaxSegmentDuration time.Duration
	// Decide is Autopilot's injected LLM curation step. Autopilot cannot
	// start without it; Freestyle and chat keepalive never use it.
	Decide DecideFunc
	// DecideSpeech runs the independent autonomous speech contract.
	DecideSpeech DecideFunc
	// CanAnnounce is false while autonomous speech would deepen a TTS backlog.
	CanAnnounce func() bool
	// Announce publishes an Autopilot line and optionally queues browser audio.
	Announce func(ctx context.Context, say string) Announcement
}

// Status is the UI-facing mode state.
type Status struct {
	Active bool   `json:"active"`
	Mode   string `json:"mode,omitempty"`
	Style  string `json:"style,omitempty"`
	// StatusAt and the absolute deadlines let a client render a smooth clock
	// between backend snapshots without inventing its own schedule. The legacy
	// remaining fields stay as a compatibility and clock-skew fallback.
	StatusAt       string `json:"status_at,omitempty"`
	SegmentIndex   int    `json:"segment_index,omitempty"`
	SegmentEndsMs  int64  `json:"segment_ends_in_ms,omitempty"`
	SegmentDueAt   string `json:"segment_due_at,omitempty"`
	LastEvent      string `json:"last_event,omitempty"`
	LastEventAt    string `json:"last_event_at,omitempty"`
	WaitingForChat bool   `json:"waiting_for_chat,omitempty"`
	// DecisionSource reports where Autopilot's current segment came from:
	// "model", "fallback" (planner after a failed decision), "hold", or
	// "interactive" when chat supplied the live target.
	DecisionSource string `json:"decision_source,omitempty"`
	// LastSay is the most recent Autopilot line. The planner uses it to avoid
	// repetition; the API retains it as diagnostic state while Chat owns display.
	LastSay string `json:"last_say,omitempty"`
	// MotionChangeMs and SpeechMs are independent backend-owned clocks.
	MotionChangeMs        int64  `json:"motion_change_in_ms,omitempty"`
	MotionChangeDueAt     string `json:"motion_change_due_at,omitempty"`
	SpeechMs              int64  `json:"speech_in_ms,omitempty"`
	SpeechDueAt           string `json:"speech_due_at,omitempty"`
	MotionPlanned         bool   `json:"motion_planned,omitempty"`
	SpeechWaitingPlayback bool   `json:"speech_waiting_playback,omitempty"`
	// Arc is the visible session progression bar. Absent when the user has the
	// switch off, so the UI shows nothing rather than an empty bar.
	Arc *SessionArc `json:"session_arc,omitempty"`
}

// Manager owns at most one active mode loop.
type Manager struct {
	lifecycleMu   sync.Mutex
	userIntentMu  sync.Mutex
	userControlMu sync.Mutex
	mu            sync.Mutex
	options       Options
	loop          modeLoopState
	user          userControlState
	chat          chatRecoveryState
	motion        motionScheduleState
	speech        speechScheduleState
	history       motionHistoryState
	events        modeEventState
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
	if options.AutopilotSettings == nil {
		options.AutopilotSettings = func() config.AutopilotSettings {
			return config.DefaultAutopilotSettings()
		}
	}
	if options.CanAnnounce == nil {
		options.CanAnnounce = func() bool { return true }
	}
	return &Manager{options: options}, nil
}

// Start activates a mode, replacing any active one.
func (m *Manager) Start(ctx context.Context, mode string) (Status, error) {
	if err := ctx.Err(); err != nil {
		return m.Status(), err
	}
	if mode != ModeFreestyle && mode != ModeAutopilot && mode != ModeChat {
		return m.Status(), fmt.Errorf("unknown mode %q", mode)
	}
	if mode == ModeAutopilot && m.options.Decide == nil {
		return m.Status(), errors.New("autopilot requires a configured decision step")
	}

	finishStart, startAdmitted := m.beginModeStart()
	defer finishStart()
	// Admission can wait behind a user control. Check again before stopping
	// the current mode, then after draining it, before detaching the new loop
	// from the HTTP request's lifetime.
	if err := ctx.Err(); err != nil {
		return m.Status(), err
	}
	if !startAdmitted {
		return m.Status(), errors.New("mode start was superseded by a newer user control")
	}
	m.stopLoop("mode_switch")
	if err := ctx.Err(); err != nil {
		return m.Status(), err
	}

	m.mu.Lock()
	m.resetForModeStartLocked(mode)
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.loop.cancel = cancel
	done := make(chan struct{})
	m.loop.done = done
	m.mu.Unlock()

	m.trace(mode, "mode_started", nil, "")
	go func() {
		defer close(done)
		m.run(loopCtx, mode)
	}()
	return m.Status(), nil
}

func (m *Manager) beginModeStart() (func(), bool) {
	// Register before waiting for physical Pause/Resume work. A newer user
	// control or Stop can then invalidate this queued start rather than letting
	// it revive motion after the newer intent.
	m.userIntentMu.Lock()
	m.mu.Lock()
	m.user.intentID++
	startID := m.user.intentID
	m.mu.Unlock()
	m.userIntentMu.Unlock()

	// Execution order is control -> lifecycle -> intent. Emergency Stop needs
	// only lifecycle and can therefore overtake a queued start, invalidate its
	// ID, and complete physical Stop without waiting for these gates.
	m.userControlMu.Lock()
	m.lifecycleMu.Lock()
	m.userIntentMu.Lock()
	m.mu.Lock()
	admitted := m.user.intentID == startID
	m.mu.Unlock()
	return func() {
		m.userIntentMu.Unlock()
		m.lifecycleMu.Unlock()
		m.userControlMu.Unlock()
	}, admitted
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
	if m.loop.mode == "" {
		m.mu.Unlock()
		return
	}
	mode := m.loop.mode
	cancel := m.loop.cancel
	done := m.loop.done
	m.loop.generation++
	m.cancelOperationLocked()
	m.resetUserPauseLocked()
	m.loop.mode = ""
	m.loop.cancel = nil
	m.loop.done = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	m.trace(mode, "mode_stopped", nil, reason)
}

// stopLoopAtGeneration is the asynchronous counterpart used when the mode loop
// itself discovers a terminal condition. Binding teardown to the failed
// generation prevents a delayed goroutine from stopping a newer user-started
// run.
func (m *Manager) stopLoopAtGeneration(mode string, generation uint64, reason string) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	m.mu.Lock()
	if m.loop.mode != mode || m.loop.generation != generation {
		m.mu.Unlock()
		return
	}
	cancel := m.loop.cancel
	done := m.loop.done
	m.loop.generation++
	m.cancelOperationLocked()
	m.resetUserPauseLocked()
	m.loop.mode = ""
	m.loop.cancel = nil
	m.loop.done = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	m.trace(mode, "mode_stopped", nil, reason)
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

// tickAutonomous routes each autonomous mode through its owned scheduler.
func (m *Manager) tickAutonomous(ctx context.Context, mode string) {
	if mode == ModeAutopilot {
		m.tickAutopilot(ctx)
		return
	}
	m.tickFreestyle(ctx, mode)
}

// tickFreestyle advances the deterministic segment and drift clocks.
func (m *Manager) tickFreestyle(ctx context.Context, mode string) {
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
	if m.freezeIfPaused(mode, snapshot.Paused) {
		return
	}
	m.thawDeadline()

	if !snapshot.Running {
		m.mu.Lock()
		stopped := m.user.stopped
		retryAt := m.motion.nextRetry
		generation := m.loop.generation
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
	deadline := m.motion.deadline
	driftAt := m.motion.driftAt
	driftDone := m.motion.driftDone
	segment := m.motion.segment
	pattern := m.motion.pattern
	retryAt := m.motion.nextRetry
	generation := m.loop.generation
	m.mu.Unlock()

	if !driftDone && now.After(driftAt) {
		operationCtx, finish, ok := m.beginStartOperation(ctx, mode, generation, 0)
		if !ok {
			return
		}
		defer finish()
		if target, ok := segment.DriftTarget(modeLabel(mode), mode); ok {
			target.Pattern = pattern
			if _, err := engine.ApplyTarget(operationCtx, target, mode+"_drift"); err == nil {
				if m.modeGenerationActive(mode, generation) {
					m.trace(mode, "segment_drift", &diagnostics.MotionTracePlanner{
						Mode:              mode,
						Event:             "segment_drift",
						PatternIdentifier: segmentContentIdentifier(segment),
						DriftToPercent:    segment.DriftToSpeedPercent,
					}, "")
				}
			}
		}
		if operationCtx.Err() != nil {
			return
		}
		m.mu.Lock()
		if m.loop.mode == mode && m.loop.generation == generation && !m.chat.pending {
			m.motion.driftDone = true
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
	if !choice.segment.hasContent() {
		detail := "model did not provide a startable motion target"
		if note := strings.TrimSpace(choice.note); note != "" {
			detail += ": " + note
		} else if choice.source == "hold" {
			detail += ": model chose continuity before any target was active"
		}
		m.backoff(mode, generation, "start_waiting_for_model", errors.New(detail))
		return
	}
	state, err := engine.Start(operationCtx, m.choiceTarget(mode, choice), m.options.Settings())
	if err != nil {
		if operationCtx.Err() != nil {
			return
		}
		m.handleStartFailure(mode, generation, "start_failed", err)
		return
	}
	choice.appliedPerceptual = clonePerceptualSummary(state.Perceptual)
	m.finishSegmentChoice(operationCtx, mode, reason, choice, state.RecentCommandLatencyMillis, generation)
}

// applyNextSegment retargets the running stream to the next segment.
// Transitions ride the engine's phase-preserving / low-jump handoff — modes
// never replace streams or touch transport.
func (m *Manager) applyNextSegment(ctx context.Context, engine Engine, mode string, reason string, generation uint64) {
	operationCtx, finish, ok := m.beginStartOperation(ctx, mode, generation, 0)
	if !ok {
		return
	}
	defer finish()

	choice := m.nextSegmentChoice(operationCtx, mode)
	if operationCtx.Err() != nil || !m.modeGenerationActive(mode, generation) {
		return
	}
	state, err := engine.ApplyTarget(operationCtx, m.choiceTarget(mode, choice), reason)
	if err != nil {
		if operationCtx.Err() == nil {
			m.backoff(mode, generation, "segment_failed", err)
		}
		return
	}
	choice.appliedPerceptual = clonePerceptualSummary(state.Perceptual)
	m.finishSegmentChoice(operationCtx, mode, reason, choice, state.RecentCommandLatencyMillis, generation)
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

// finishSegmentChoice arms a first/recovered segment. Autopilot uses its own
// cadence scheduler; Freestyle keeps the segment-duration clock.
func (m *Manager) finishSegmentChoice(_ context.Context, mode string, reason string, choice segmentChoice, recentLatencyMillis int64, generation uint64) {
	if mode == ModeAutopilot {
		if !m.armAutopilotChoice(mode, &choice, generation) {
			return
		}
		m.rememberChoice(mode, choice)
		m.tracePlanned(mode, reason, choice)
		return
	}
	if !m.armSegment(mode, choice.segment, choice.pattern, recentLatencyMillis, generation) {
		return
	}
	m.rememberChoice(mode, choice)
	m.tracePlanned(mode, reason, choice)
}

func (m *Manager) nextPlannedSegment() (Segment, []diagnostics.PlannerScore) {
	m.mu.Lock()
	planner := m.motion.planner
	m.mu.Unlock()
	if planner == nil {
		planner = NewPlanner(m.options.Seed)
		m.mu.Lock()
		m.motion.planner = planner
		m.mu.Unlock()
	}
	return planner.NextSegment(m.options.Settings())
}

func (m *Manager) armSegment(mode string, segment Segment, pattern *motion.PatternDefinition, recentLatencyMillis int64, generation uint64) bool {
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
	if m.loop.mode != mode || m.loop.generation != generation || m.user.stopped || m.user.paused || m.chat.pending {
		return false
	}
	m.motion.segment = segment
	m.motion.pattern = pattern
	m.motion.segmentIdx++
	m.motion.deadline = now.Add(duration)
	if segment.DriftToSpeedPercent != 0 {
		m.motion.driftAt = now.Add(duration / 2)
		m.motion.driftDone = false
	} else {
		m.motion.driftDone = true
	}
	m.motion.nextRetry = time.Time{}
	return true
}

func (m *Manager) modeActive(mode string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loop.mode == mode && !m.user.stopped && !m.chat.pending &&
		(mode != ModeAutopilot || !m.chat.activity)
}

func (m *Manager) modeGenerationActive(mode string, generation uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loop.mode == mode && m.loop.generation == generation && !m.user.stopped && !m.user.paused &&
		!m.chat.pending && (mode != ModeAutopilot || !m.chat.activity)
}

func (m *Manager) userPauseActive(mode string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loop.mode == mode && m.user.paused
}

func (m *Manager) freezeIfPaused(mode string, enginePaused bool) bool {
	if !enginePaused && !m.userPauseActive(mode) {
		return false
	}
	m.freezeDeadline()
	return true
}

func (m *Manager) freezeDeadline() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.loop.wasPaused {
		m.loop.wasPaused = true
	}
	// Shift the clock forward every paused tick so remaining time is intact.
	m.motion.postpone(m.options.Tick)
	m.speech.postpone(m.options.Tick)
	m.history.postpone(m.options.Tick)
}

func (m *Manager) thawDeadline() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loop.wasPaused = false
}

func (m *Manager) backoff(mode string, generation uint64, event string, err error) {
	m.mu.Lock()
	if m.loop.mode != mode || m.loop.generation != generation || m.user.stopped || m.chat.pending {
		m.mu.Unlock()
		return
	}
	m.motion.nextRetry = m.options.Now().Add(restartBackoff)
	m.mu.Unlock()
	m.trace(mode, event, nil, err.Error())
}

// handleStartFailure keeps transient transport/model failures retryable while
// opening the circuit for physical geometry that the shared motion engine has
// already classified as unsafe. Retrying unchanged unsafe state only repeats
// Stop and read-only Cloud calls, flashes the UI between starting and idle, and
// cannot make the slider position or calibration become safer.
func (m *Manager) handleStartFailure(mode string, generation uint64, event string, err error) {
	if !errors.Is(err, motion.ErrUnsafeStartupState) {
		m.backoff(mode, generation, event, err)
		return
	}

	m.mu.Lock()
	active := m.loop.mode == mode && m.loop.generation == generation && !m.user.stopped && !m.chat.pending
	m.mu.Unlock()
	if !active {
		return
	}
	m.trace(mode, event, nil, err.Error())
	// Stop waits for the mode loop, so it must run after this tick returns to the
	// loop rather than waiting on its own goroutine.
	go m.stopLoopAtGeneration(mode, generation, "unsafe_startup_state")
}

func (m *Manager) beginStartOperation(
	parent context.Context,
	mode string,
	generation uint64,
	chatVersion uint64,
) (context.Context, func(), bool) {
	m.mu.Lock()
	if m.loop.mode != mode || m.loop.generation != generation || m.user.stopped || m.user.paused || m.chat.pending ||
		(mode == ModeAutopilot && m.chat.activity) ||
		(mode == ModeChat && (m.chat.version != chatVersion || m.chat.target == nil || !m.chat.keepalive)) {
		m.mu.Unlock()
		return nil, nil, false
	}
	operationCtx, cancel := context.WithCancel(parent)
	m.loop.operationID++
	id := m.loop.operationID
	m.loop.operationMode = mode
	m.loop.operationCancel = cancel
	m.mu.Unlock()

	return operationCtx, func() {
		cancel()
		m.mu.Lock()
		if m.loop.operationID == id {
			m.loop.operationMode = ""
			m.loop.operationCancel = nil
		}
		m.mu.Unlock()
	}, true
}

func (m *Manager) cancelOperationLocked() {
	if m.loop.operationCancel != nil {
		m.loop.operationCancel()
	}
}

func cloneTarget(target motion.MotionTarget) motion.MotionTarget {
	target.Flow = motion.CloneFlowSpec(target.Flow)
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
	if target.Dynamic != nil {
		target.Dynamic = cloneDynamicDefinition(target.Dynamic)
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
	m.mu.Lock()
	segmentIndex := m.motion.segmentIdx
	m.mu.Unlock()
	row := &diagnostics.MotionTracePlanner{
		Mode:              mode,
		Event:             reason,
		Style:             m.options.Settings().Style,
		PatternIdentifier: segmentContentIdentifier(choice.segment),
		SpeedPercent:      choice.segment.SpeedPercent,
		DriftToPercent:    choice.segment.DriftToSpeedPercent,
		DurationMillis:    choice.segment.DurationMillis,
		Scores:            choice.scores,
		SegmentIndex:      segmentIndex,
	}
	if planner != nil {
		row.Seed = planner.Seed()
	}
	note := choice.note
	if mode == ModeAutopilot {
		// Preserve source, semantic timing, and inference latency so a sampled
		// dwell can be explained after either a planned or immediate decision.
		note = strings.TrimSpace(fmt.Sprintf(
			"%s %s next=%s latency=%s%s",
			choice.source,
			note,
			normalizeTiming(choice.timing),
			choice.decisionLatency,
			choice.sessionTraceNote(),
		))
		if choice.say != "" {
			note += " say"
		}
	}
	m.trace(mode, reason, row, note)
}

func segmentContentIdentifier(segment Segment) string {
	if segment.Flow != nil {
		if segment.Flow.Gesture != nil {
			return "creative_v2"
		}
		return "layered"
	}
	if segment.Dynamic != nil {
		return "dynamic"
	}
	return string(segment.PatternID)
}

func (m *Manager) plannerSnapshot() *Planner {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.motion.planner
}

func (m *Manager) trace(mode string, event string, planner *diagnostics.MotionTracePlanner, note string) {
	m.mu.Lock()
	m.events.lastEvent = event
	m.events.lastEventAt = m.options.Now()
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
