package modes

import "time"

// resetForModeStartLocked initializes a newly admitted mode after the previous
// loop has drained. Keep state initialization separate from admission and loop
// ownership so cancellation checks remain visible in Start. Caller holds m.mu.
func (m *Manager) resetForModeStartLocked(mode string) {
	m.loop.generation++
	m.chat.version++
	m.loop.mode = mode
	m.resetUserPauseLocked()
	m.user.stopped = false
	m.chat.target = nil
	m.chat.keepalive = false
	m.chat.pending = false
	m.motion.driftDone = true
	m.motion.swayPoints = nil
	m.history.previousSpeed = 0
	m.history.speedChangedAt = time.Time{}
	m.history.currentPhrase = Segment{}
	m.history.currentPerceptual = nil
	m.history.phraseChangedAt = time.Time{}
	m.history.decisionsAtCurrentPhrase = 0
	m.history.consecutiveHolds = 0
	// A new run is a new arc: the bar measures this session, not the last one.
	m.history.arc = arcState{startedAt: m.options.Now()}
	m.motion.deadline = time.Time{}
	m.motion.nextRetry = time.Time{}
	if mode == ModeFreestyle || mode == ModeAutopilot {
		m.motion.planner = NewPlanner(m.options.Seed)
		m.motion.segmentIdx = 0
		m.motion.segment = Segment{}
		m.motion.pattern = nil
		m.history.recentPatternIDs = nil
		m.history.recentPositionBands = nil
		m.events.decisionSource = ""
		m.speech.lastSay = ""
		m.motion.planAt = time.Time{}
		m.motion.pending = nil
		m.speech.deadline = time.Time{}
		m.speech.waitingID = ""
		m.speech.fallbackAt = time.Time{}
		m.speech.nextTiming = TimingNormal
		m.motion.lastDecisionTime = 0
		m.motion.cadenceRNG = nil
		m.speech.cadenceRNG = nil
	}
}
