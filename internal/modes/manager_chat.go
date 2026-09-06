package modes

import (
	"context"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// PrepareChatTarget blocks new mode work and invalidates any in-flight decision
// before an interactive target enters the shared engine.
func (m *Manager) PrepareChatTarget() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chat.pending = true
	m.loop.generation++
	m.cancelOperationLocked()
	m.motion.swayPoints = nil
	return m.loop.generation
}

// NotifyChatActivity invalidates stale autonomous planning and postpones the
// independent speech clock even when the interactive turn does not change
// motion.
func (m *Manager) NotifyChatActivity() uint64 {
	now := m.options.Now()
	m.mu.Lock()
	activityID := m.chat.beginActivity()
	if m.loop.mode != ModeAutopilot || m.user.stopped {
		m.mu.Unlock()
		return activityID
	}
	m.loop.generation++
	m.cancelOperationLocked()
	m.motion.pending = nil
	m.motion.swayPoints = nil
	m.speech.waitingID = ""
	m.speech.fallbackAt = time.Time{}
	m.scheduleSpeechLocked(now, TimingNormal)
	m.mu.Unlock()
	m.trace(ModeAutopilot, "chat_activity", nil, "autonomous speech postponed")
	return activityID
}

// NotifyChatActivityComplete releases autonomous planning after the canonical
// owning interactive turn has finished. An invalidated turn may unwind after
// its replacement has started, so completion must carry its activity ID.
func (m *Manager) NotifyChatActivityComplete(activityID uint64) {
	m.mu.Lock()
	m.chat.completeActivity(activityID)
	m.mu.Unlock()
}

// CancelChatTarget releases a failed interactive handoff so the active mode can
// resume planning on its next tick.
func (m *Manager) CancelChatTarget(generation uint64) {
	m.mu.Lock()
	if m.chat.pending && m.loop.generation == generation {
		m.chat.pending = false
	}
	m.mu.Unlock()
}

// NotifyChatTarget adopts a successfully applied chat target for keepalive and
// as Autopilot's authoritative current segment.
func (m *Manager) NotifyChatTarget(generation uint64, target motion.MotionTarget) bool {
	copied := cloneTarget(target)
	// Re-evaluate an interactive target on the independent motion cadence.
	segment, pattern, adoptable := segmentFromMotionTarget(copied, 0)
	now := m.options.Now()
	var perceptual *motion.PerceptualSummary
	if engine := m.options.Current(); engine != nil {
		perceptual = clonePerceptualSummary(engine.Snapshot().Perceptual)
	}

	m.mu.Lock()
	if !m.chat.pending || m.loop.generation != generation || m.user.stopped {
		m.mu.Unlock()
		return false
	}
	m.chat.pending = false
	m.chat.version++
	m.loop.generation++
	m.cancelOperationLocked()
	m.chat.target = &copied
	// Only reusable loop patterns are recovery targets. Programs and media are
	// finite; an idle engine means they completed rather than lost transport.
	m.chat.keepalive = adoptable
	adopted := m.loop.mode == ModeAutopilot && adoptable
	if adopted {
		previousSpeed := m.motion.segment.SpeedPercent
		duration := m.sampleMotionDelayLocked(TimingSoon)
		if m.options.MaxSegmentDuration > 0 && duration > m.options.MaxSegmentDuration {
			duration = m.options.MaxSegmentDuration
		}
		segment.DurationMillis = duration.Milliseconds()
		m.motion.segment = segment
		m.motion.pattern = pattern
		m.motion.segmentIdx++
		m.motion.deadline = now.Add(duration)
		m.motion.planAt = m.motion.deadline.Add(-m.planningLeadLocked(duration))
		m.motion.pending = nil
		m.motion.swayPoints = nil
		m.motion.driftDone = true
		m.motion.nextRetry = time.Time{}
		if segment.SpeedPercent != previousSpeed {
			m.history.previousSpeed = previousSpeed
			m.history.speedChangedAt = now
		}
		m.observeInteractivePhraseLocked(now, segment, perceptual)
		m.rememberPositionBandLocked(perceptual)
		m.events.decisionSource = "interactive"
		if segment.PatternID != "" {
			m.history.recentPatternIDs = append(m.history.recentPatternIDs, string(segment.PatternID))
			if len(m.history.recentPatternIDs) > 4 {
				m.history.recentPatternIDs = m.history.recentPatternIDs[len(m.history.recentPatternIDs)-4:]
			}
		}
	}
	m.mu.Unlock()

	if adopted {
		m.trace(ModeAutopilot, "interactive_target_adopted", &diagnostics.MotionTracePlanner{
			Mode:              ModeAutopilot,
			Event:             "interactive_target_adopted",
			PatternIdentifier: segmentContentIdentifier(segment),
			SpeedPercent:      segment.SpeedPercent,
			DurationMillis:    segment.DurationMillis,
		}, "chat")
	}
	return true
}

// NotifyChatStop clears the keepalive target after a chat-driven stop.
func (m *Manager) NotifyChatStop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetUserPauseLocked()
	m.chat.target = nil
	m.chat.keepalive = false
	m.chat.pending = false
	m.chat.activity = false
	m.user.stopped = true
	m.chat.version++
	m.loop.generation++
	m.cancelOperationLocked()
}

func (m *Manager) tickChat(ctx context.Context) {
	if ctx.Err() != nil || !m.modeActive(ModeChat) {
		return
	}
	if m.userPauseActive(ModeChat) {
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
	if m.chat.target != nil && m.chat.keepalive {
		copied := cloneTarget(*m.chat.target)
		target = &copied
	}
	stopped := m.user.stopped
	retryAt := m.motion.nextRetry
	generation := m.loop.generation
	chatVersion := m.chat.version
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
		m.handleStartFailure(ModeChat, generation, "keepalive_failed", err)
		return
	}
	if !m.chatOperationActive(generation, chatVersion) {
		return
	}
	m.trace(ModeChat, "chat_keepalive_restart", &diagnostics.MotionTracePlanner{
		Mode:  ModeChat,
		Event: "chat_keepalive_restart",
		PatternIdentifier: func() string {
			if target.Dynamic != nil {
				return "dynamic"
			}
			return string(target.PatternID)
		}(),
		SpeedPercent: target.SpeedPercent,
	}, "")
}

func (m *Manager) chatOperationActive(generation, chatVersion uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loop.mode == ModeChat && m.loop.generation == generation && m.chat.version == chatVersion &&
		m.chat.target != nil && m.chat.keepalive && !m.user.stopped && !m.user.paused && !m.chat.pending
}
