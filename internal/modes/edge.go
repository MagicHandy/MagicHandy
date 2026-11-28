package modes

import (
	"context"
	"errors"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// EdgeInput describes the mechanical state after an immediate holding override.
// Only the normal mode loop may schedule its automatic return.
type EdgeInput struct {
	ChatSessionID string    `json:"-"`
	AfterSeq      int64     `json:"-"`
	PressedAt     time.Time `json:"-"`
	TempoOnly     bool      `json:"tempo_only,omitempty"`
	PreviousSpeed int       `json:"previous_speed_percent"`
	HoldingSpeed  int       `json:"holding_speed_percent"`
	SpeedMin      int       `json:"speed_min_percent"`
	SpeedMax      int       `json:"resume_speed_max_percent"`
	RangeMin      int       `json:"holding_range_min_percent"`
	RangeMax      int       `json:"holding_range_max_percent"`
}

// EdgeDecision chooses a bounded duration and return, never raw device commands.
type EdgeDecision struct {
	HoldSeconds int    `json:"hold_seconds"`
	ResumeSpeed int    `json:"resume_speed_percent"`
	ResumeStyle string `json:"resume_style"`
}

// Validate rejects incomplete or unsafe model decisions.
func (d EdgeDecision) Validate(input EdgeInput) error {
	if d.HoldSeconds < 8 || d.HoldSeconds > 180 || d.ResumeSpeed < input.SpeedMin || d.ResumeSpeed > input.SpeedMax || (d.ResumeStyle != "previous" && d.ResumeStyle != "gentle") {
		return errors.New("holding plan is outside its permitted duration, pace or return style")
	}
	return nil
}

// EdgeStatus is the backend-owned holding control snapshot.
type EdgeStatus struct {
	ChatSessionID   string    `json:"-"`
	PressedAt       time.Time `json:"-"`
	Active          bool      `json:"active"`
	Phase           string    `json:"phase"`
	Automatic       bool      `json:"automatic"`
	ResumeAt        string    `json:"resume_at,omitempty"`
	ResumeInSeconds int       `json:"resume_in_seconds,omitempty"`
	Message         string    `json:"message,omitempty"`
}

type edgeState struct {
	status    EdgeStatus
	previous  motion.MotionTarget
	holding   motion.MotionTarget
	planID    string
	input     EdgeInput
	decision  *EdgeDecision
	resumeAt  time.Time
	applied   bool
	keepalive bool
}

type guardedEdgeEngine interface {
	Engine
	ApplyTargetIfCurrent(context.Context, motion.MotionTarget, string, string) (motion.ActiveMotionState, error)
}

// ActivateEdge does no model work and never starts a stopped or paused device.
// The plan-ID guard and engine Stop generation protect the physical handoff.
func (m *Manager) ActivateEdge(ctx context.Context) (Status, error) {
	return m.ActivateChatEdge(ctx, "", 0)
}

// ActivateChatEdge binds observations to the conversation active at the press.
func (m *Manager) ActivateChatEdge(ctx context.Context, sessionID string, afterSeq int64) (Status, error) {
	m.userControlMu.Lock()
	defer m.userControlMu.Unlock()
	if err := ctx.Err(); err != nil {
		return m.Status(), err
	}
	m.reconcileEdge()
	engine, ok := m.options.Current().(guardedEdgeEngine)
	if !ok {
		return m.Status(), errors.New("start motion before using Edge")
	}
	before := engine.Snapshot()
	if !before.AllowsMovingHold() {
		return m.Status(), motion.ErrEdgeUnavailable
	}
	if repeated, err := m.latchActiveEdge(ctx, before.PlanID); repeated {
		if err == nil && m.options.EdgeAdmitted != nil {
			m.options.EdgeAdmitted()
		}
		return m.Status(), err
	}
	settings := m.options.Settings()
	holding, err := motion.HoldingTarget(before, settings)
	if err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return m.Status(), err
	}
	if m.user.paused {
		m.mu.Unlock()
		return m.Status(), errors.New("a hold or pause is already active")
	}
	m.loop.generation++
	generation := m.loop.generation
	m.cancelOperationLocked()
	keepalive := m.chat.keepalive
	m.chat.pending, m.chat.keepalive = false, false
	m.chat.version++
	m.motion.pending, m.motion.swayPoints = nil, nil
	m.motion.deadline, m.motion.planAt = time.Time{}, time.Time{}
	m.speech.deadline, m.speech.waitingID = time.Time{}, ""
	automatic := m.loop.mode == ModeAutopilot && m.options.DecideEdge != nil
	e := &edgeState{status: EdgeStatus{Active: true, Phase: "applying", Automatic: automatic}, previous: cloneTarget(before.Target), holding: holding, keepalive: keepalive}
	e.input = edgeInput(before, holding, settings.SpeedMinPercent, settings.SpeedMaxPercent)
	e.input.ChatSessionID, e.input.AfterSeq, e.input.PressedAt = sessionID, afterSeq, m.options.Now()
	e.status.ChatSessionID, e.status.PressedAt = sessionID, e.input.PressedAt
	m.edge = e
	m.events.decisionSource = "edge_hold"
	m.mu.Unlock()
	if m.options.EdgeAdmitted != nil {
		m.options.EdgeAdmitted()
	}
	state, err := engine.ApplyTargetIfCurrent(ctx, holding, "edge_hold", before.PlanID)
	if err == nil && m.options.EdgeApplied != nil {
		m.options.EdgeApplied(ctx, e.input)
	}
	m.mu.Lock()
	if m.edge == e {
		m.finishEdgeApplyLocked(e, before.PlanID, generation, state, err)
	}
	m.mu.Unlock()
	return m.Status(), err
}

func (m *Manager) finishEdgeApplyLocked(e *edgeState, previousPlan string, generation uint64, state motion.ActiveMotionState, err error) {
	if err != nil {
		if state.Running && state.Target.Source == "edge" && state.PlanID != previousPlan {
			e.applied, e.planID = true, state.PlanID
			m.suspendEdgeLocked("Holding until you choose Return.")
			return
		}
		m.edge = nil
		if !state.Running && !state.Paused {
			m.user.stopped = true
			m.chat.target, m.chat.keepalive = nil, false
		} else if state.PlanID == previousPlan && m.loop.generation == generation && !m.user.stopped {
			m.chat.keepalive = e.keepalive
		}
		return
	}
	e.applied, e.planID = true, state.PlanID
	e.status.Phase = "holding"
	if m.loop.generation != generation {
		m.suspendEdgeLocked("The session changed. Resume when ready.")
	} else if e.status.Automatic {
		e.status.Phase = "planning"
	}
}

// A second press on the same running hold revokes its automatic Return.
func (m *Manager) latchActiveEdge(ctx context.Context, planID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.edge == nil || !m.edge.status.Active {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	if m.user.paused || m.edge.planID != planID {
		return true, motion.ErrEdgeUnavailable
	}
	m.loop.generation++
	m.cancelOperationLocked()
	m.suspendEdgeLocked("Holding until you choose Return.")
	return true, nil
}

// suspendEdgeLocked preserves the gentle motion but revokes automatic return
// whenever a newer user intent or configuration invalidates its assumptions.
func (m *Manager) suspendEdgeLocked(message string) {
	if m.edge == nil {
		return
	}
	m.edge.status.Automatic = false
	m.edge.status.Phase, m.edge.status.Message = "holding", message
	m.edge.resumeAt, m.edge.status.ResumeAt = time.Time{}, ""
}

// tickEdge consumes the ordinary scheduler tick while the override is active.
// No recovery Start is permitted after a transport failure in a holding session.
func (m *Manager) tickEdge(ctx context.Context) bool {
	m.reconcileEdge()
	m.mu.Lock()
	e := m.edge
	if e == nil {
		m.mu.Unlock()
		return false
	}
	generation := m.loop.generation
	m.mu.Unlock()
	engine := m.options.Current()
	if engine == nil {
		m.mu.Lock()
		if m.edge == e {
			m.edge = nil
			m.user.stopped = true
			m.chat.keepalive = false
		}
		m.mu.Unlock()
		return true
	}
	state := engine.Snapshot()
	m.mu.Lock()
	if m.edge != e {
		m.mu.Unlock()
		return true
	}
	if !e.applied || state.Paused || m.user.paused || m.chat.pending || m.chat.activity {
		m.mu.Unlock()
		return true
	}
	automatic, decision, resumeAt := e.status.Automatic, e.decision, e.resumeAt
	m.mu.Unlock()
	if !automatic {
		return true
	}
	if decision == nil {
		m.planEdgeReturn(ctx, e, generation)
		return true
	}
	if !m.options.Now().Before(resumeAt) {
		_, _ = m.resumeEdge(ctx, e, generation, true)
	}
	return true
}

func (m *Manager) planEdgeReturn(ctx context.Context, e *edgeState, generation uint64) {
	operation, finish, ok := m.beginModeOperation(ctx, ModeAutopilot, generation, 0, true)
	if !ok {
		return
	}
	defer finish()
	bounded, cancel := context.WithTimeout(operation, 25*time.Second)
	defer cancel()
	plan, err := m.options.DecideEdge(bounded, e.input)
	if err == nil {
		err = plan.Validate(e.input)
	}
	accepted := false
	m.mu.Lock()
	if m.edge == e && m.loop.generation == generation && operation.Err() == nil && e.status.Automatic {
		if err != nil {
			m.suspendEdgeLocked("Autopilot could not plan the return. Resume when ready.")
		} else {
			e.decision = &plan
			e.resumeAt = m.options.Now().Add(time.Duration(plan.HoldSeconds) * time.Second)
			e.status.Phase, e.status.ResumeAt = "holding", e.resumeAt.UTC().Format(time.RFC3339Nano)
			accepted = true
		}
	}
	m.mu.Unlock()
	if accepted {
		if m.options.EdgePlanned != nil {
			m.options.EdgePlanned(operation, e.input, plan)
		}
		m.trace(ModeAutopilot, "edge_return_planned", &diagnostics.MotionTracePlanner{Mode: ModeAutopilot, Event: "edge_return_planned", SpeedPercent: plan.ResumeSpeed, DurationMillis: int64(plan.HoldSeconds) * 1000}, plan.ResumeStyle)
	} else if err != nil && operation.Err() == nil {
		m.trace(ModeAutopilot, "edge_return_unavailable", nil, "gentle hold retained; manual return required")
	}
}

// ResumeEdge returns to the saved motion without starting a stopped device.
func (m *Manager) ResumeEdge(ctx context.Context) (Status, error) {
	m.mu.Lock()
	e, generation := m.edge, m.loop.generation
	m.mu.Unlock()
	return m.resumeEdge(ctx, e, generation, false)
}

func (m *Manager) resumeEdge(ctx context.Context, expected *edgeState, generation uint64, automatic bool) (Status, error) {
	if automatic {
		if !m.userControlMu.TryLock() {
			return m.Status(), nil
		}
	} else {
		m.userControlMu.Lock()
	}
	defer m.userControlMu.Unlock()
	m.mu.Lock()
	if expected == nil || m.edge != expected || generation != m.loop.generation || !expected.applied || m.user.paused || (automatic && !expected.status.Automatic) {
		m.mu.Unlock()
		return m.Status(), errors.New("the holding session changed; use the current controls")
	}
	e := m.edge
	target := edgeReturnTarget(e, automatic)
	m.loop.generation++
	resumeGeneration := m.loop.generation
	m.cancelOperationLocked()
	operation, cancel := context.WithCancel(ctx)
	m.loop.operationID++
	operationID := m.loop.operationID
	m.loop.operationCancel = cancel
	planID := e.planID
	m.suspendEdgeLocked("")
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		if m.loop.operationID == operationID {
			m.loop.operationCancel = nil
		}
		m.mu.Unlock()
	}()
	engine, ok := m.options.Current().(guardedEdgeEngine)
	if !ok {
		return m.Status(), errors.New("motion is no longer available")
	}
	state, err := engine.ApplyTargetIfCurrent(operation, target, "edge_resume", planID)
	m.mu.Lock()
	if m.edge == e {
		if err == nil {
			m.edge = nil
			if m.loop.generation == resumeGeneration {
				m.adoptEdgeReturnLocked(e, state)
			}
		} else {
			m.suspendEdgeLocked("The return could not be applied. Motion was not restarted.")
		}
	}
	m.mu.Unlock()
	return m.Status(), err
}

func edgeReturnTarget(e *edgeState, automatic bool) motion.MotionTarget {
	target := cloneTarget(e.previous)
	if target.TempoOnly {
		if automatic && e.decision.ResumeStyle == "gentle" {
			target = cloneTarget(e.holding)
			target.Tempo = &motion.TempoSpec{Mode: motion.TempoModeSteady, LevelPercent: e.decision.ResumeSpeed}
		}
		if automatic {
			target.Source, target.Label = ModeAutopilot, "Autopilot"
		}
		return target
	}
	if automatic {
		if e.decision.ResumeStyle == "gentle" {
			target = cloneTarget(e.holding)
		}
		// Steps and specialised scores carry their own pace. Do not pretend a
		// scalar edit slows them: use the known gentle curve for this return.
		if target.Flow != nil && (len(target.Flow.Steps) > 0 || target.Flow.Freestyle != nil || target.Flow.Strokes != nil) {
			target = cloneTarget(e.holding)
		}
		target.SpeedPercent = e.decision.ResumeSpeed
		target.Source, target.Label = ModeAutopilot, "Autopilot"
	}
	return target
}

func edgeInput(before motion.ActiveMotionState, holding motion.MotionTarget, speedMin, speedMax int) EdgeInput {
	if holding.TempoOnly {
		previous := max(1, min(100, int(before.Tempo.LevelPercent)))
		return EdgeInput{TempoOnly: true, PreviousSpeed: previous, HoldingSpeed: holding.Tempo.LevelPercent, SpeedMin: 1, SpeedMax: previous}
	}
	return EdgeInput{PreviousSpeed: before.Target.SpeedPercent, HoldingSpeed: holding.SpeedPercent,
		SpeedMin: speedMin, SpeedMax: min(before.Target.SpeedPercent, speedMax),
		RangeMin: holding.Flow.MinPercent, RangeMax: holding.Flow.MaxPercent}
}

func (m *Manager) adoptEdgeReturnLocked(e *edgeState, state motion.ActiveMotionState) {
	copied := cloneTarget(state.Target)
	segment, pattern, adoptable := segmentFromMotionTarget(copied, 0)
	m.chat.target, m.chat.keepalive = &copied, adoptable && e.keepalive
	m.motion.segment, m.motion.pattern = segment, pattern
	now := m.options.Now()
	duration := m.sampleMotionDelayLocked(TimingNormal)
	m.motion.segment.DurationMillis = duration.Milliseconds()
	m.motion.deadline, m.motion.planAt = now.Add(duration), m.planAtLocked(now, duration)
	m.motion.pending, m.motion.swayPoints = nil, nil
	m.motion.nextRetry = time.Time{}
	m.history.previousSpeed, m.history.speedChangedAt = e.holding.SpeedPercent, now
	m.rememberSpeedLocked(copied.SpeedPercent, now, false)
	perceptual := clonePerceptualSummary(state.Perceptual)
	m.observeAutopilotPhraseLocked(now, segmentChoice{segment: m.motion.segment, appliedPerceptual: perceptual, source: "edge_return"})
	m.rememberPositionBandLocked(perceptual)
	m.scheduleSpeechLocked(now, TimingNormal)
	m.events.decisionSource = "edge_return"
}

// Manual holds have no scheduler loop. Reconcile observations as well as ticks
// so their UI cannot claim a hold after a disconnect or an external target.
func (m *Manager) reconcileEdge() {
	m.mu.Lock()
	expected := m.edge
	if expected == nil || !expected.applied || m.user.paused {
		m.mu.Unlock()
		return
	}
	// A status read can overlap installation or replacement of a plan.
	// Only reconcile an engine snapshot taken for this same applied hold.
	planID, generation := expected.planID, m.loop.generation
	m.mu.Unlock()
	engine := m.options.Current()
	var state motion.ActiveMotionState
	if engine != nil {
		state = engine.Snapshot()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.edge
	if e != expected || e.planID != planID || m.loop.generation != generation || m.user.paused || state.Paused {
		return
	}
	if !state.Running {
		m.edge, m.chat.target, m.chat.keepalive = nil, nil, false
		m.user.stopped = true
		m.cancelOperationLocked()
		return
	}
	if state.PlanID == e.planID {
		return
	}
	if state.Target.Source == "edge" {
		e.planID, e.holding = state.PlanID, cloneTarget(state.Target)
		m.suspendEdgeLocked("Hold updated. Choose Return when ready.")
	} else {
		m.edge = nil
		// A chat handoff has its own final adoption and history update. A
		// status observation between apply and notify must not adopt it twice.
		if !m.chat.pending {
			m.adoptEdgeReturnLocked(e, state)
		}
	}
}
