package modes

// Only a current motion-planning result changes this counter. A spoken
// check-in or result already invalidated by chat/Stop is not evidence that
// autonomous motion planning recovered (or failed).
func (m *Manager) recordPlanningOutcome(choice segmentChoice, generation uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loop.mode != ModeAutopilot || m.loop.generation != generation || m.chat.pending || m.user.stopped {
		return
	}
	if choice.failed {
		m.events.planningFailures = min(m.events.planningFailures+1, 255)
	} else {
		m.events.planningFailures = 0
	}
}
