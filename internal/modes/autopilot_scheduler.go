package modes

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	defaultPlanningLead       = 3 * time.Second
	minimumPlanningLead       = 1 * time.Second
	maximumPlanningLead       = 10 * time.Second
	speechBacklogRetry        = 5 * time.Second
	speechDecisionRetry       = 15 * time.Second
	speechPlaybackAckFallback = 2 * time.Minute
	motionCadenceSeedSalt     = int64(0x5deece66d)
	speechCadenceSeedSalt     = int64(0x6a09e667f3bcc909)
)

// tickAutopilot advances independent motion and speech clocks. Motion work is
// always considered first so a due spoken check-in cannot delay a target
// boundary.
func (m *Manager) tickAutopilot(ctx context.Context) {
	if ctx.Err() != nil || !m.modeActive(ModeAutopilot) {
		return
	}
	engine := m.options.Current()
	var snapshot motion.ActiveMotionState
	if engine != nil {
		snapshot = engine.Snapshot()
	}
	if snapshot.Paused {
		m.freezeDeadline()
		return
	}
	m.thawDeadline()

	if !snapshot.Running {
		m.mu.Lock()
		stopped := m.userStopped
		retryAt := m.nextRetry
		generation := m.generation
		m.mu.Unlock()
		if stopped {
			go m.Stop("user_stop_observed")
			return
		}
		if m.options.Now().Before(retryAt) {
			return
		}
		m.startNextSegment(ctx, ModeAutopilot, "autopilot_start", generation)
		return
	}

	now := m.options.Now()
	m.mu.Lock()
	pending := m.pendingMotion
	deadline := m.deadline
	planAt := m.motionPlanAt
	retryAt := m.nextRetry
	generation := m.generation
	waitingID := m.speechWaitingID
	fallbackAt := m.speechFallbackAt
	m.mu.Unlock()

	if now.Before(retryAt) {
		return
	}
	if pending != nil && !now.Before(deadline) {
		m.applyPendingAutopilotChoice(ctx, engine, generation)
		return
	}
	if pending == nil && (planAt.IsZero() || !now.Before(planAt)) {
		m.planAutopilotMotion(ctx, engine, generation)
		return
	}

	// Sway is texture inside the live segment, so it is applied after boundary
	// work but before speech, and it never moves the segment deadline.
	if point, ok := m.dueSway(now, generation); ok {
		m.applyDueSway(ctx, engine, generation, point)
		return
	}

	if waitingID != "" {
		if !fallbackAt.IsZero() && !now.Before(fallbackAt) {
			m.completeSpeechWait(waitingID, "playback_ack_timeout")
		}
		return
	}
	m.tickAutopilotSpeech(ctx, engine, generation)
}

func (m *Manager) planAutopilotMotion(ctx context.Context, engine Engine, generation uint64) {
	operationCtx, finish, ok := m.beginStartOperation(ctx, ModeAutopilot, generation, 0)
	if !ok {
		return
	}
	defer finish()

	choice := m.nextSegmentChoice(operationCtx, ModeAutopilot)
	if operationCtx.Err() != nil || !m.modeGenerationActive(ModeAutopilot, generation) {
		return
	}
	now := m.options.Now()
	m.mu.Lock()
	if m.mode != ModeAutopilot || m.generation != generation || m.chatTargetPending || m.userStopped {
		m.mu.Unlock()
		return
	}
	m.lastDecisionTime = choice.decisionLatency
	if now.Before(m.deadline) {
		copied := choice
		m.pendingMotion = &copied
		m.mu.Unlock()
		m.trace(ModeAutopilot, "motion_planned", nil,
			fmt.Sprintf("%s next=%s variability=%s latency=%s",
				choice.source, choice.timing, normalizeVariability(choice.variability), choice.decisionLatency))
		return
	}
	m.mu.Unlock()
	m.applyAutopilotChoice(operationCtx, engine, choice, generation, "autopilot_segment")
}

func (m *Manager) applyPendingAutopilotChoice(ctx context.Context, engine Engine, generation uint64) {
	operationCtx, finish, ok := m.beginStartOperation(ctx, ModeAutopilot, generation, 0)
	if !ok {
		return
	}
	defer finish()

	m.mu.Lock()
	if m.mode != ModeAutopilot || m.generation != generation || m.pendingMotion == nil {
		m.mu.Unlock()
		return
	}
	choice := *m.pendingMotion
	m.pendingMotion = nil
	m.mu.Unlock()
	m.applyAutopilotChoice(operationCtx, engine, choice, generation, "autopilot_segment")
}

func (m *Manager) applyAutopilotChoice(
	ctx context.Context,
	engine Engine,
	choice segmentChoice,
	generation uint64,
	reason string,
) {
	if choice.source == "hold" {
		if !m.armAutopilotChoice(ModeAutopilot, &choice, generation) {
			return
		}
		m.rememberChoice(ModeAutopilot, choice)
		m.tracePlanned(ModeAutopilot, "autopilot_hold", choice)
		return
	}
	state, err := engine.ApplyTarget(ctx, m.choiceTarget(ModeAutopilot, choice), reason)
	if err != nil {
		if ctx.Err() == nil {
			m.backoff(ModeAutopilot, generation, "segment_failed", err)
			m.mu.Lock()
			if m.mode == ModeAutopilot && m.generation == generation {
				m.pendingMotion = nil
				m.deadline = m.nextRetry
				m.motionPlanAt = m.nextRetry
			}
			m.mu.Unlock()
		}
		return
	}
	if !m.armAutopilotChoice(ModeAutopilot, &choice, generation) {
		return
	}
	m.rememberChoice(ModeAutopilot, choice)
	m.tracePlanned(ModeAutopilot, reason, choice)
	if state.RecentCommandLatencyMillis > 0 {
		m.mu.Lock()
		if m.mode == ModeAutopilot && m.generation == generation {
			transportLatency := time.Duration(state.RecentCommandLatencyMillis) * time.Millisecond
			if transportLatency > m.lastDecisionTime {
				m.lastDecisionTime = transportLatency
			}
		}
		m.mu.Unlock()
	}
}

func (m *Manager) armAutopilotChoice(mode string, choice *segmentChoice, generation uint64) bool {
	now := m.options.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mode != mode || m.generation != generation || m.userStopped || m.chatTargetPending {
		return false
	}
	previousSpeed := m.segment.SpeedPercent
	if choice.decisionLatency > 0 {
		m.lastDecisionTime = choice.decisionLatency
	}
	duration := m.sampleMotionDelayLocked(choice.timing)
	if choice.segment.Dynamic != nil && choice.segment.Dynamic.SegmentSeconds > 0 {
		requested := time.Duration(choice.segment.Dynamic.SegmentSeconds) * time.Second
		settings := m.options.AutopilotSettings()
		minimum := time.Duration(settings.MotionMinSeconds) * time.Second
		maximum := time.Duration(settings.MotionMaxSeconds) * time.Second
		if minimum > 0 && requested < minimum {
			requested = minimum
		}
		if maximum >= minimum && requested > maximum {
			requested = maximum
		}
		duration = requested
	}
	if m.options.MaxSegmentDuration > 0 && duration > m.options.MaxSegmentDuration {
		duration = m.options.MaxSegmentDuration
	}
	choice.segment.DurationMillis = duration.Milliseconds()
	m.segment = choice.segment
	m.pattern = choice.pattern
	m.segmentIdx++
	m.deadline = now.Add(duration)
	m.motionPlanAt = m.deadline.Add(-m.planningLeadLocked(duration))
	m.pendingMotion = nil
	// tickAutopilot never read driftAt/driftDone, so the old midpoint step was
	// write-only state here. Intra-segment variation is now the sway schedule.
	m.driftDone = true
	if choice.segment.SpeedPercent != previousSpeed {
		m.previousSpeed = previousSpeed
		m.speedChangedAt = now
	}
	m.swayPoints = m.planSwayLocked(now, duration, *choice, generation)
	m.nextRetry = time.Time{}
	if m.speechDeadline.IsZero() && m.speechWaitingID == "" {
		m.scheduleSpeechLocked(now, TimingNormal)
	}
	return true
}

func (m *Manager) tickAutopilotSpeech(ctx context.Context, engine Engine, generation uint64) {
	if m.options.DecideSpeech == nil || m.options.Announce == nil {
		return
	}
	now := m.options.Now()
	m.mu.Lock()
	deadline := m.speechDeadline
	m.mu.Unlock()
	if deadline.IsZero() || now.Before(deadline) {
		return
	}
	if !m.options.CanAnnounce() {
		m.retryAutopilotSpeech(generation, speechBacklogRetry, "speech_postponed", "voice backlog")
		return
	}

	operationCtx, finish, ok := m.beginStartOperation(ctx, ModeAutopilot, generation, 0)
	if !ok {
		return
	}
	defer finish()
	choice := m.runDecision(operationCtx, m.options.DecideSpeech, false)
	if operationCtx.Err() != nil || !m.modeGenerationActive(ModeAutopilot, generation) {
		return
	}
	if choice.source == "speech_error" || choice.say == "" {
		m.retryAutopilotSpeech(generation, speechDecisionRetry, "speech_failed", choice.note)
		return
	}

	announcement := m.options.Announce(operationCtx, choice.say)
	if operationCtx.Err() != nil || !m.modeGenerationActive(ModeAutopilot, generation) {
		return
	}
	if !announcement.Published {
		m.retryAutopilotSpeech(generation, speechDecisionRetry, "speech_publish_failed", "")
		return
	}
	// Canonical Chat publication admits the coupled action. Applying motion
	// first could leave an unexplained device change when persistence or speech
	// publication failed.
	m.applyAutopilotSpeechMotion(operationCtx, engine, generation, choice)
	if operationCtx.Err() != nil || !m.modeGenerationActive(ModeAutopilot, generation) {
		return
	}
	m.armAutopilotSpeech(generation, choice.say, choice.timing, announcement)
}

func (m *Manager) retryAutopilotSpeech(generation uint64, delay time.Duration, event string, note string) {
	m.mu.Lock()
	if m.mode != ModeAutopilot || m.generation != generation || m.userStopped {
		m.mu.Unlock()
		return
	}
	m.speechDeadline = m.options.Now().Add(delay)
	m.mu.Unlock()
	m.trace(ModeAutopilot, event, nil, note)
}

func (m *Manager) applyAutopilotSpeechMotion(
	ctx context.Context,
	engine Engine,
	generation uint64,
	choice segmentChoice,
) {
	if choice.source == "hold" || !choice.segment.hasContent() {
		return
	}
	motionChoice := choice
	motionChoice.source = "speech"
	motionChoice.timing = TimingNormal
	if _, err := engine.ApplyTarget(
		ctx,
		m.choiceTarget(ModeAutopilot, motionChoice),
		"autopilot_speech",
	); err != nil {
		if ctx.Err() == nil {
			m.trace(ModeAutopilot, "speech_motion_failed", nil, err.Error())
		}
		return
	}
	if m.armAutopilotChoice(ModeAutopilot, &motionChoice, generation) {
		m.rememberChoice(ModeAutopilot, motionChoice)
		m.tracePlanned(ModeAutopilot, "autopilot_speech", motionChoice)
	}
}

func (m *Manager) armAutopilotSpeech(
	generation uint64,
	say string,
	timing TimingPreference,
	announcement Announcement,
) {
	m.mu.Lock()
	if m.mode != ModeAutopilot || m.generation != generation || m.userStopped {
		m.mu.Unlock()
		return
	}
	m.lastSay = say
	m.speechNextTiming = timing
	if announcement.AwaitPlayback && announcement.RequestID != "" {
		m.speechDeadline = time.Time{}
		m.speechWaitingID = announcement.RequestID
		m.speechFallbackAt = m.options.Now().Add(speechPlaybackAckFallback)
	} else {
		m.scheduleSpeechLocked(m.options.Now(), timing)
	}
	m.mu.Unlock()
	m.trace(ModeAutopilot, "speech_published", nil,
		fmt.Sprintf("next=%s playback=%t", timing, announcement.AwaitPlayback))
}

// NotifySpeechPlaybackComplete starts the next speech interval after audible
// playback, not after inference or enqueue completion.
func (m *Manager) NotifySpeechPlaybackComplete(requestID string) bool {
	return m.completeSpeechWait(requestID, "playback_complete")
}

func (m *Manager) completeSpeechWait(requestID string, reason string) bool {
	m.mu.Lock()
	if m.mode != ModeAutopilot || requestID == "" || m.speechWaitingID != requestID {
		m.mu.Unlock()
		return false
	}
	timing := m.speechNextTiming
	m.speechWaitingID = ""
	m.speechFallbackAt = time.Time{}
	m.scheduleSpeechLocked(m.options.Now(), timing)
	m.mu.Unlock()
	m.trace(ModeAutopilot, reason, nil, fmt.Sprintf("next=%s", timing))
	return true
}

// NotifyAutopilotSettingsChanged applies saved cadence preferences to the live
// session without retargeting current motion.
func (m *Manager) NotifyAutopilotSettingsChanged() {
	now := m.options.Now()
	m.mu.Lock()
	if m.mode != ModeAutopilot || m.userStopped {
		m.mu.Unlock()
		return
	}
	m.generation++
	m.cancelOperationLocked()
	m.pendingMotion = nil
	m.swayPoints = nil
	m.deadline = now.Add(m.sampleMotionDelayLocked(TimingNormal))
	m.motionPlanAt = m.deadline.Add(-m.planningLeadLocked(m.deadline.Sub(now)))
	m.speechWaitingID = ""
	m.speechFallbackAt = time.Time{}
	m.scheduleSpeechLocked(now, TimingNormal)
	m.mu.Unlock()
	m.trace(ModeAutopilot, "cadence_settings_changed", nil, "")
}

func (m *Manager) scheduleSpeechLocked(now time.Time, timing TimingPreference) {
	settings := m.options.AutopilotSettings()
	_, _, enabled := settings.SpeechWindow()
	if !enabled || m.options.DecideSpeech == nil || m.options.Announce == nil {
		m.speechDeadline = time.Time{}
		return
	}
	m.speechDeadline = now.Add(m.sampleSpeechDelayLocked(timing))
}

func (m *Manager) sampleMotionDelayLocked(timing TimingPreference) time.Duration {
	settings := m.options.AutopilotSettings()
	minimum, maximum := settings.MotionWindow()
	m.ensureCadenceRNGsLocked()
	return m.sampleCadenceLocked(
		minimum,
		maximum,
		timing,
		settings.AdaptiveMotionTiming,
		m.motionCadenceRNG,
	)
}

func (m *Manager) sampleSpeechDelayLocked(timing TimingPreference) time.Duration {
	settings := m.options.AutopilotSettings()
	minimum, maximum, enabled := settings.SpeechWindow()
	if !enabled {
		return 0
	}
	m.ensureCadenceRNGsLocked()
	return m.sampleCadenceLocked(
		minimum,
		maximum,
		timing,
		settings.AdaptiveSpeechTiming,
		m.speechCadenceRNG,
	)
}

func (m *Manager) sampleCadenceLocked(
	minimumSeconds int,
	maximumSeconds int,
	timing TimingPreference,
	adaptive bool,
	rng *rand.Rand,
) time.Duration {
	if minimumSeconds > maximumSeconds {
		minimumSeconds, maximumSeconds = maximumSeconds, minimumSeconds
	}
	if minimumSeconds < config.AutopilotMinimumIntervalSeconds {
		minimumSeconds = config.AutopilotMinimumIntervalSeconds
	}
	low, high := minimumSeconds, maximumSeconds
	if adaptive && maximumSeconds > minimumSeconds {
		span := maximumSeconds - minimumSeconds
		switch normalizeTiming(timing) {
		case TimingSoon:
			high = minimumSeconds + (span*55)/100
		case TimingLater:
			low = minimumSeconds + (span*45)/100
		default:
			low = minimumSeconds + (span*20)/100
			high = minimumSeconds + (span*80)/100
		}
	}
	if high < low {
		high = low
	}
	seconds := low
	if spread := high - low; spread > 0 {
		seconds += rng.Intn(spread + 1)
	}
	return time.Duration(seconds) * time.Second
}

func (m *Manager) ensureCadenceRNGsLocked() {
	if m.planner == nil {
		m.planner = NewPlanner(m.options.Seed)
	}
	seed := m.planner.Seed()
	if m.motionCadenceRNG == nil {
		m.motionCadenceRNG = newCadenceRNG(seed, motionCadenceSeedSalt)
	}
	if m.speechCadenceRNG == nil {
		m.speechCadenceRNG = newCadenceRNG(seed, speechCadenceSeedSalt)
	}
}

func newCadenceRNG(seed int64, salt int64) *rand.Rand {
	//nolint:gosec // Reproducible cadence variation, never security material.
	return rand.New(rand.NewSource(seed ^ salt))
}

func (m *Manager) planningLeadLocked(duration time.Duration) time.Duration {
	lead := m.lastDecisionTime + 500*time.Millisecond
	if m.lastDecisionTime <= 0 {
		lead = defaultPlanningLead
	}
	if lead < minimumPlanningLead {
		lead = minimumPlanningLead
	}
	if lead > maximumPlanningLead {
		lead = maximumPlanningLead
	}
	if maximum := duration / 2; maximum > 0 && lead > maximum {
		lead = maximum
	}
	return lead
}
