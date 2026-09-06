package modes

import "time"

// resetForModeStartLocked initializes a newly admitted mode after the previous
// loop has drained. Keep state initialization separate from admission and loop
// ownership so cancellation checks remain visible in Start. Caller holds m.mu.
func (m *Manager) resetForModeStartLocked(mode string) {
	m.generation++
	m.chatVersion++
	m.mode = mode
	m.resetUserPauseLocked()
	m.userStopped = false
	m.chatTarget = nil
	m.chatKeepalive = false
	m.chatTargetPending = false
	m.driftDone = true
	m.swayPoints = nil
	m.previousSpeed = 0
	m.speedChangedAt = time.Time{}
	m.currentPhrase = Segment{}
	m.currentPerceptual = nil
	m.phraseChangedAt = time.Time{}
	m.decisionsAtCurrentPhrase = 0
	m.consecutiveHolds = 0
	// A new run is a new arc: the bar measures this session, not the last one.
	m.arc = arcState{startedAt: m.options.Now()}
	m.deadline = time.Time{}
	m.nextRetry = time.Time{}
	if mode == ModeFreestyle || mode == ModeAutopilot {
		m.planner = NewPlanner(m.options.Seed)
		m.segmentIdx = 0
		m.segment = Segment{}
		m.pattern = nil
		m.recentPatternIDs = nil
		m.recentPositionBands = nil
		m.decisionSource = ""
		m.lastSay = ""
		m.motionPlanAt = time.Time{}
		m.pendingMotion = nil
		m.speechDeadline = time.Time{}
		m.speechWaitingID = ""
		m.speechFallbackAt = time.Time{}
		m.speechNextTiming = TimingNormal
		m.lastDecisionTime = 0
		m.motionCadenceRNG = nil
		m.speechCadenceRNG = nil
	}
}
