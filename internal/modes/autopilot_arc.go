package modes

import (
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// The session arc is a visible fill bar the model is encouraged to build along.
//
// A model-maintained score that quietly accumulates and then drives intensity is
// the hidden-escalation shape docs/goals-and-guardrails.md rules out. Four
// properties are what make this version safe instead, and all four are load
// bearing:
//
//   - Visible. The value is rendered, so nothing about the progression is hidden
//     from the person it is happening to.
//   - User-armed. Off is the default, and off removes the arc from the prompt
//     entirely rather than sending a zero — the model cannot act on a field it
//     never saw, which is the same discipline the capability gates use.
//   - Bounded. It is a percentage with a full mark, not a counter that grows.
//   - Backend-owned. The model may ask to advance or ease by one clamped step; it
//     can never write the value. So it cannot sprint the bar to full, and the
//     trace shows every nudge.
//
// What the arc does *not* do is widen anything. It positions intent inside the
// user's existing speed band. Speed limits, focus range, and capability gates
// stay exactly where the user set them, and the engine clamps regardless.
type arcState struct {
	// percent is the current fill, 0-100.
	percent int
	// startedAt anchors the time-driven component.
	startedAt time.Time
	// lastNudge records the most recent model intent for the trace.
	lastNudge string
}

// SessionArc is the UI-facing arc snapshot.
type SessionArc struct {
	Enabled bool   `json:"enabled"`
	Percent int    `json:"percent"`
	Minutes int    `json:"minutes"`
	Intent  string `json:"intent,omitempty"`
}

// arcPercentLocked resolves the fill from elapsed time and accumulated nudges.
// Time is the floor so the bar always progresses for a user who is simply
// letting a session run; nudges let the model lead or lag that baseline.
// Callers hold the lock.
func (m *Manager) arcPercentLocked(now time.Time) int {
	settings := m.options.AutopilotSettings()
	if !settings.SessionArc || !settings.SessionTracking {
		return 0
	}
	minutes := settings.SessionArcMinutes
	if minutes < config.AutopilotMinimumArcMinutes {
		minutes = config.AutopilotDefaultArcMinutes
	}
	if m.arc.startedAt.IsZero() {
		m.arc.startedAt = now
	}
	elapsed := now.Sub(m.arc.startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	byTime := int(elapsed * 100 / (time.Duration(minutes) * time.Minute))
	percent := byTime
	if m.arc.percent > percent {
		percent = m.arc.percent
	}
	return clampInt(percent, 0, 100)
}

// applyArcIntentLocked moves the arc by at most one clamped step. Callers hold
// the lock.
func (m *Manager) applyArcIntentLocked(now time.Time, intent string) {
	settings := m.options.AutopilotSettings()
	if !settings.SessionArc || !settings.SessionTracking {
		return
	}
	if !config.ValidAutopilotArcIntent(intent) {
		intent = config.AutopilotArcHold
	}
	current := m.arcPercentLocked(now)
	m.arc.lastNudge = intent
	switch intent {
	case config.AutopilotArcAdvance:
		m.arc.percent = clampInt(current+config.AutopilotArcNudgePercent, 0, 100)
	case config.AutopilotArcEase:
		// Easing below the time baseline is allowed: winding down is a legitimate
		// direction, and the next arcPercentLocked floor keeps it from sticking
		// at zero for the rest of a long session.
		m.arc.percent = clampInt(current-config.AutopilotArcNudgePercent, 0, 100)
	default:
		m.arc.percent = current
	}
}

// SessionArcSnapshot reports the arc for the UI.
func (m *Manager) SessionArcSnapshot() SessionArc {
	settings := m.options.AutopilotSettings()
	m.mu.Lock()
	defer m.mu.Unlock()
	arc := SessionArc{
		Enabled: settings.SessionArc && settings.SessionTracking,
		Minutes: settings.SessionArcMinutes,
		Intent:  m.arc.lastNudge,
	}
	if arc.Enabled && m.mode == ModeAutopilot {
		arc.Percent = m.arcPercentLocked(m.options.Now())
	}
	return arc
}

// ResetSessionArc returns the bar to empty. The user owns the arc as much as the
// model does; a bar you cannot pull back is not really an override.
//
// It reports false when there is no Autopilot session to place an arc in. Start
// clears the arc for a fresh run, so accepting a placement beforehand would store
// a value that is silently discarded a moment later — a call that appears to work
// and does nothing is worse than one that says no.
func (m *Manager) ResetSessionArc() bool {
	m.mu.Lock()
	if m.mode != ModeAutopilot {
		m.mu.Unlock()
		return false
	}
	m.arc = arcState{startedAt: m.options.Now()}
	m.mu.Unlock()
	m.trace(ModeAutopilot, "session_arc_reset", nil, "")
	return true
}

// SetSessionArcPercent lets the user place the bar directly.
func (m *Manager) SetSessionArcPercent(percent int) bool {
	now := m.options.Now()
	m.mu.Lock()
	if m.mode != ModeAutopilot {
		m.mu.Unlock()
		return false
	}
	m.arc.percent = clampInt(percent, 0, 100)
	// Re-anchor the time baseline to the placed value so the bar does not snap
	// back to wherever elapsed time had already reached.
	settings := m.options.AutopilotSettings()
	minutes := settings.SessionArcMinutes
	if minutes < config.AutopilotMinimumArcMinutes {
		minutes = config.AutopilotDefaultArcMinutes
	}
	offset := time.Duration(m.arc.percent) * time.Duration(minutes) * time.Minute / 100
	m.arc.startedAt = now.Add(-offset)
	m.mu.Unlock()
	m.trace(ModeAutopilot, "session_arc_set", nil, "")
	return true
}
