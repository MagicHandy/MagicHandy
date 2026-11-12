package modes

import (
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Session buildup is visible progress the model is encouraged to build along.
//
// A model-maintained score that quietly accumulates and then drives intensity is
// the hidden-escalation shape docs/goals-and-guardrails.md rules out. Four
// properties are what make this version safe instead, and all four are load
// bearing:
//
//   - Visible. The value is rendered, so nothing about the progression is hidden
//     from the person it is happening to.
//   - User-armed. Off is the default, and off removes buildup from the prompt
//     entirely rather than sending a zero — the model cannot act on a field it
//     never saw, which is the same discipline the capability gates use.
//   - Bounded. It is a percentage with a full mark, not a counter that grows.
//   - Backend-owned. Active elapsed time is the only automatic input. The model
//     can react to the percentage but cannot accelerate, rewind, or write it.
//
// What buildup does *not* do is widen anything. It positions intent inside the
// user's existing speed band. Speed limits, focus range, and capability gates
// stay exactly where the user set them, and the engine clamps regardless.
type arcState struct {
	// startedAt anchors elapsed progress. Model decisions never rewrite it; the
	// configured duration is therefore the actual time from empty to full.
	startedAt time.Time
}

// SessionArc is the UI-facing buildup snapshot. Its name preserves the API schema.
type SessionArc struct {
	Enabled bool `json:"enabled"`
	Percent int  `json:"percent"`
	Minutes int  `json:"minutes"`
}

// arcPercentLocked resolves fill strictly from active elapsed time. The model
// may react to this backend-owned value but cannot accelerate or rewind it.
// Callers hold the lock.
func (m *Manager) arcPercentLocked(now time.Time) int {
	settings := m.options.AutopilotSettings()
	if !settings.SessionArc {
		return 0
	}
	minutes := arcMinutes(settings)
	if m.history.arc.startedAt.IsZero() {
		m.history.arc.startedAt = now
	}
	elapsed := now.Sub(m.history.arc.startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	// Millisecond scaling keeps the documented duration range exact enough for a
	// percentage while avoiding elapsed*100 overflowing time.Duration.
	elapsedUnits := elapsed / time.Millisecond
	durationUnits := time.Duration(minutes) * time.Minute / time.Millisecond
	byTime := int(elapsedUnits * 100 / durationUnits)
	return clampInt(byTime, 0, 100)
}

// placeArcPercentLocked moves the visible value and re-anchors elapsed progress
// at that same point. Callers hold the lock.
func (m *Manager) placeArcPercentLocked(now time.Time, percent int) {
	percent = clampInt(percent, 0, 100)
	minutes := arcMinutes(m.options.AutopilotSettings())
	// Scale in milliseconds, as in arcPercentLocked: percent*duration would
	// overflow for durations the settings validator explicitly permits.
	durationUnits := time.Duration(minutes) * time.Minute / time.Millisecond
	offset := durationUnits * time.Duration(percent) / 100 * time.Millisecond
	m.history.arc.startedAt = now.Add(-offset)
}

// arcMinutes is the buildup's configured length, with the first-run default
// standing in for a value below the minimum.
func arcMinutes(settings config.AutopilotSettings) int {
	if settings.SessionArcMinutes < config.AutopilotMinimumArcMinutes {
		return config.AutopilotDefaultArcMinutes
	}
	return settings.SessionArcMinutes
}

// SessionArcSnapshot reports buildup for the UI.
func (m *Manager) SessionArcSnapshot() SessionArc {
	settings := m.options.AutopilotSettings()
	m.mu.Lock()
	defer m.mu.Unlock()
	arc := SessionArc{
		Enabled: settings.SessionArc,
		Minutes: settings.SessionArcMinutes,
	}
	if arc.Enabled && m.loop.mode == ModeAutopilot {
		arc.Percent = m.arcPercentLocked(m.options.Now())
	}
	return arc
}

// ResetSessionArc returns buildup to empty. A visible progression the user cannot
// restart is not really under user control.
//
// It reports false when there is no Autopilot session to place buildup in. Start
// clears buildup for a fresh run, so accepting a placement beforehand would store
// a value that is silently discarded a moment later — a call that appears to work
// and does nothing is worse than one that says no.
func (m *Manager) ResetSessionArc() bool {
	m.mu.Lock()
	if m.loop.mode != ModeAutopilot {
		m.mu.Unlock()
		return false
	}
	now := m.options.Now()
	m.history.arc = arcState{startedAt: now}
	m.buildupChangedLocked(now)
	m.mu.Unlock()
	m.trace(ModeAutopilot, "session_arc_reset", nil, "")
	return true
}

// SetSessionArcPercent lets the user place the bar directly.
func (m *Manager) SetSessionArcPercent(percent int) bool {
	now := m.options.Now()
	m.mu.Lock()
	if m.loop.mode != ModeAutopilot {
		m.mu.Unlock()
		return false
	}
	// Re-anchor the time baseline to the placed value so the bar does not snap
	// back to wherever elapsed time had already reached.
	m.placeArcPercentLocked(now, percent)
	m.buildupChangedLocked(now)
	m.mu.Unlock()
	m.trace(ModeAutopilot, "session_arc_set", nil, "")
	return true
}

// A prepared decision belongs to the old finish target. Discard it atomically
// with the visible edit and ask for a fresh decision, without changing current
// playback or releasing Pause/Stop/chat admission.
func (m *Manager) buildupChangedLocked(now time.Time) {
	m.loop.generation++
	m.cancelOperationLocked()
	m.motion.pending = nil
	m.motion.swayPoints = nil
	m.motion.nextRetry = time.Time{}
	m.motion.deadline, m.motion.planAt = now, now
}
