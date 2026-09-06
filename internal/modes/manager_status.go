package modes

import "time"

// Status returns the UI-facing mode state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	mode := m.loop.mode
	lastEvent := m.events.lastEvent
	lastEventAt := m.events.lastEventAt
	segmentIdx := m.motion.segmentIdx
	deadline := m.motion.deadline
	waitingForChat := m.chat.target == nil
	decisionSource := m.events.decisionSource
	lastSay := m.speech.lastSay
	pendingMotion := m.motion.pending != nil
	speechDeadline := m.speech.deadline
	speechWaiting := m.speech.waitingID != ""
	m.mu.Unlock()
	now := m.options.Now()

	status := Status{
		Active:    mode != "",
		Mode:      mode,
		LastEvent: lastEvent,
	}
	if mode != "" {
		status.Style = m.options.Settings().Style
		status.StatusAt = now.UTC().Format(time.RFC3339Nano)
	}
	if !lastEventAt.IsZero() {
		status.LastEventAt = lastEventAt.UTC().Format(time.RFC3339Nano)
	}
	if mode == ModeFreestyle || mode == ModeAutopilot {
		status.SegmentIndex = segmentIdx
		if !deadline.IsZero() {
			status.SegmentDueAt = deadline.UTC().Format(time.RFC3339Nano)
		}
		if remaining := deadline.Sub(now).Milliseconds(); remaining > 0 {
			status.SegmentEndsMs = remaining
		}
	}
	if mode == ModeAutopilot {
		status.DecisionSource = decisionSource
		status.LastSay = lastSay
		status.MotionPlanned = pendingMotion
		status.SpeechWaitingPlayback = speechWaiting
		if arc := m.SessionArcSnapshot(); arc.Enabled {
			status.Arc = &arc
		}
		if !deadline.IsZero() {
			status.MotionChangeDueAt = deadline.UTC().Format(time.RFC3339Nano)
		}
		if remaining := deadline.Sub(now).Milliseconds(); remaining > 0 {
			status.MotionChangeMs = remaining
		}
		if !speechDeadline.IsZero() {
			status.SpeechDueAt = speechDeadline.UTC().Format(time.RFC3339Nano)
		}
		if remaining := speechDeadline.Sub(now).Milliseconds(); remaining > 0 {
			status.SpeechMs = remaining
		}
	}
	if mode == ModeChat {
		status.WaitingForChat = waitingForChat
	}
	return status
}
