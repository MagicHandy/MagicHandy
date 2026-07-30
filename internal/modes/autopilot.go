package modes

import (
	"context"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const decisionTimeout = 25 * time.Second

// TimingPreference is a model-selected semantic timing category. The manager
// samples the concrete delay inside the user's saved bounds.
type TimingPreference string

const (
	// TimingSoon samples the early part of the configured cadence window.
	TimingSoon TimingPreference = "soon"
	// TimingNormal samples the middle of the configured cadence window.
	TimingNormal TimingPreference = "normal"
	// TimingLater samples the late part of the configured cadence window.
	TimingLater TimingPreference = "later"
)

// DecisionInput is bounded, semantic context for one autonomous decision.
type DecisionInput struct {
	Style            string
	SegmentIndex     int
	RecentPatternIDs []string
	SpeedMinPercent  int
	SpeedMaxPercent  int
	LastSay          string
	CurrentPatternID motion.PatternID
	CurrentSpeed     int
	CurrentAreaFocus *motion.AreaFocus
}

// Decision is one motion or speech curation outcome. Hold is scheduler-only:
// it must never produce an engine retarget.
type Decision struct {
	Segment Segment
	Pattern *motion.PatternDefinition
	Say     string
	Hold    bool
	Next    TimingPreference
}

// Announcement reports whether a line entered canonical chat and whether the
// scheduler must wait for browser playback before starting the next interval.
type Announcement struct {
	Published     bool
	RequestID     string
	AwaitPlayback bool
}

// DecideFunc produces one bounded autonomous decision.
type DecideFunc func(ctx context.Context, input DecisionInput) (Decision, error)

// segmentChoice is what one autonomous motion boundary applies.
type segmentChoice struct {
	segment         Segment
	pattern         *motion.PatternDefinition
	scores          []diagnostics.PlannerScore
	source          string // planner, model, fallback, hold, speech
	note            string
	say             string
	timing          TimingPreference
	decisionLatency time.Duration
}

// nextSegmentChoice picks the next segment for Freestyle or Autopilot.
func (m *Manager) nextSegmentChoice(ctx context.Context, mode string) segmentChoice {
	if mode != ModeAutopilot || m.options.Decide == nil {
		segment, scores := m.nextPlannedSegment()
		return segmentChoice{segment: segment, scores: scores, source: "planner", timing: TimingNormal}
	}
	return m.runDecision(ctx, m.options.Decide, true)
}

// runDecision executes one model decision. Motion decisions fall back to the
// deterministic planner; speech decisions return their error to the speech
// scheduler so they can be postponed without disturbing motion.
func (m *Manager) runDecision(ctx context.Context, decide DecideFunc, fallback bool) segmentChoice {
	input := m.decisionInput()
	decideCtx, cancel := context.WithTimeout(ctx, decisionTimeout)
	started := m.options.Now()
	decision, err := decide(decideCtx, input)
	cancel()
	latency := m.options.Now().Sub(started)
	if latency < 0 {
		latency = 0
	}
	if err != nil {
		if !fallback {
			return segmentChoice{source: "speech_error", note: err.Error(), decisionLatency: latency}
		}
		segment, scores := m.nextPlannedSegment()
		segment.DriftToSpeedPercent = 0
		return segmentChoice{
			segment: segment, scores: scores, source: "fallback", note: err.Error(),
			timing: TimingNormal, decisionLatency: latency,
		}
	}
	timing := normalizeTiming(decision.Next)
	if decision.Hold {
		if segment, pattern, ok := m.heldSegment(); ok {
			return segmentChoice{
				segment: segment, pattern: pattern, source: "hold",
				say: decision.Say, timing: timing, decisionLatency: latency,
			}
		}
		if !fallback {
			return segmentChoice{
				source: "hold", say: decision.Say, timing: timing, decisionLatency: latency,
			}
		}
		segment, scores := m.nextPlannedSegment()
		segment.DriftToSpeedPercent = 0
		return segmentChoice{
			segment: segment, scores: scores, source: "fallback",
			note: "hold_without_segment", say: decision.Say, timing: timing, decisionLatency: latency,
		}
	}
	return segmentChoice{
		segment: NormalizeSegment(decision.Segment), pattern: decision.Pattern,
		source: "model", say: decision.Say, timing: timing, decisionLatency: latency,
	}
}

func normalizeTiming(timing TimingPreference) TimingPreference {
	switch timing {
	case TimingSoon, TimingNormal, TimingLater:
		return timing
	default:
		return TimingNormal
	}
}

// decisionInput snapshots the bounded model-visible context.
func (m *Manager) decisionInput() DecisionInput {
	settings := m.options.Settings()
	var current *motion.MotionTarget
	if engine := m.options.Current(); engine != nil {
		snapshot := engine.Snapshot()
		if (snapshot.Running || snapshot.Paused) && snapshot.Target.PatternID != "" {
			copied := cloneTarget(snapshot.Target)
			current = &copied
		}
	}

	m.mu.Lock()
	input := DecisionInput{
		Style:            settings.Style,
		SegmentIndex:     m.segmentIdx,
		RecentPatternIDs: append([]string(nil), m.recentPatternIDs...),
		SpeedMinPercent:  settings.SpeedMinPercent,
		SpeedMaxPercent:  settings.SpeedMaxPercent,
		LastSay:          m.lastSay,
	}
	if current == nil && m.segment.PatternID != "" {
		fallback := m.segment.Target(modeLabel(m.mode), m.mode)
		fallback.Pattern = m.pattern
		copied := cloneTarget(fallback)
		current = &copied
	}
	m.mu.Unlock()

	if current != nil {
		input.CurrentPatternID = current.PatternID
		input.CurrentSpeed = current.SpeedPercent
		if current.AreaFocus != nil {
			focus := *current.AreaFocus
			input.CurrentAreaFocus = &focus
		}
	}
	return input
}

func (m *Manager) heldSegment() (Segment, *motion.PatternDefinition, bool) {
	if engine := m.options.Current(); engine != nil {
		snapshot := engine.Snapshot()
		if snapshot.Running || snapshot.Paused {
			if segment, pattern, ok := segmentFromMotionTarget(snapshot.Target, 0); ok {
				return segment, pattern, true
			}
		}
	}

	m.mu.Lock()
	target := m.segment.Target(modeLabel(m.mode), m.mode)
	target.Pattern = m.pattern
	target = cloneTarget(target)
	m.mu.Unlock()
	return segmentFromMotionTarget(target, 0)
}

func segmentFromMotionTarget(target motion.MotionTarget, durationMillis int64) (Segment, *motion.PatternDefinition, bool) {
	if target.PatternID == "" || target.SpeedPercent <= 0 {
		return Segment{}, nil, false
	}
	copied := cloneTarget(target)
	segment := NormalizeSegment(Segment{
		PatternID:      copied.PatternID,
		SpeedPercent:   copied.SpeedPercent,
		AreaFocus:      copied.AreaFocus,
		DurationMillis: durationMillis,
	})
	return segment, copied.Pattern, true
}

// rememberChoice records motion provenance and recent changed patterns. Holds
// do not pollute recency because no new content was played.
func (m *Manager) rememberChoice(mode string, choice segmentChoice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mode != mode {
		return
	}
	if mode == ModeAutopilot {
		m.decisionSource = choice.source
	}
	if choice.source == "hold" || choice.segment.PatternID == "" {
		return
	}
	m.recentPatternIDs = append(m.recentPatternIDs, string(choice.segment.PatternID))
	if len(m.recentPatternIDs) > 4 {
		m.recentPatternIDs = m.recentPatternIDs[len(m.recentPatternIDs)-4:]
	}
}

func modeLabel(mode string) string {
	if mode == ModeAutopilot {
		return "Autopilot"
	}
	return "Freestyle"
}
