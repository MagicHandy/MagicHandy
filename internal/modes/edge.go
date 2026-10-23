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
	PreviousSpeed int `json:"previous_speed_percent"`
	HoldingSpeed  int `json:"holding_speed_percent"`
	SpeedMin      int `json:"speed_min_percent"`
	SpeedMax      int `json:"resume_speed_max_percent"`
	RangeMin      int `json:"holding_range_min_percent"`
	RangeMax      int `json:"holding_range_max_percent"`
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
	Active          bool   `json:"active"`
	Phase           string `json:"phase"`
	Automatic       bool   `json:"automatic"`
	ResumeAt        string `json:"resume_at,omitempty"`
	ResumeInSeconds int    `json:"resume_in_seconds,omitempty"`
	Message         string `json:"message,omitempty"`
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
	m.userControlMu.Lock()
	defer m.userControlMu.Unlock()
	m.reconcileEdge()
	m.mu.Lock()
	if m.edge != nil && m.edge.status.Active {
		m.loop.generation++
		m.cancelOperationLocked()
		m.suspendEdgeLocked("Holding until you choose Return.")
		m.mu.Unlock()
		return m.Status(), nil
	}
	m.mu.Unlock()
	engine, ok := m.options.Current().(guardedEdgeEngine)
	if !ok {
		return m.Status(), errors.New("start motion before using Edge")
	}
	before := engine.Snapshot()
	if !before.Running || before.Paused || before.Target.MediaID != "" || before.Target.ProgramID != "" {
		return m.Status(), errors.New("start live motion to use Edge; video and timed programs are not supported")
	}
	holding, err := motion.HoldingTarget(before, m.options.Settings())
	if err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
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
	e.input = EdgeInput{PreviousSpeed: before.Target.SpeedPercent, HoldingSpeed: holding.SpeedPercent,
		SpeedMin: m.options.Settings().SpeedMinPercent, SpeedMax: min(before.Target.SpeedPercent, m.options.Settings().SpeedMaxPercent),
		RangeMin: holding.Flow.MinPercent, RangeMax: holding.Flow.MaxPercent}
	m.edge = e
	m.events.decisionSource = "edge_hold"
	m.mu.Unlock()
	state, err := engine.ApplyTargetIfCurrent(ctx, holding, "edge_hold", before.PlanID)
	m.mu.Lock()
	if m.edge == e {
		if err != nil {
			m.suspendEdgeLocked("The hold could not be applied. Use Stop if needed.")
			e.status.Active, e.status.Phase = false, "failed"
			if state.Target.Source == "edge" && state.PlanID != before.PlanID {
				e.applied, e.planID = true, state.PlanID
			}
		} else {
			e.applied, e.planID = true, state.PlanID
			e.status.Phase = "holding"
			if m.loop.generation != generation {
				m.suspendEdgeLocked("The session changed. Resume when ready.")
			}
			if e.status.Automatic {
				e.status.Phase = "planning"
			}
		}
	}
	m.mu.Unlock()
	return m.Status(), err
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
	m.mu.Unlock()
	if expected == nil {
		return
	}
	engine := m.options.Current()
	var state motion.ActiveMotionState
	if engine != nil {
		state = engine.Snapshot()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.edge
	if e != expected || !e.applied || m.user.paused || state.Paused {
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
		m.suspendEdgeLocked("Motion limits changed. Choose Return when ready.")
	} else {
		m.edge = nil
		m.adoptEdgeReturnLocked(e, state)
	}
}
