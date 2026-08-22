package modes

import (
	"context"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
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

// VariabilityPreference is a model-selected category for how much the motion
// should wander *inside* the current target, as distinct from how often the
// target is reconsidered. The two are independent: a persona can hold one
// pattern for a long stretch while still breathing within it, or change target
// often while each stretch stays flat.
//
// Like TimingPreference it is a category rather than a number, because a local
// model emits a category reliably and the backend samples the concrete values —
// so a model that always answers "normal" still produces varied motion.
type VariabilityPreference string

const (
	// VariabilitySettled holds the target flat until the next boundary.
	VariabilitySettled VariabilityPreference = "settled"
	// VariabilityNormal takes about half the earned waypoint allowance.
	VariabilityNormal VariabilityPreference = "normal"
	// VariabilityRestless takes the full allowance at a wider amplitude.
	VariabilityRestless VariabilityPreference = "restless"
)

// SpeedTrend summarizes where speed has been going, so the model can answer
// "have I been sitting still?" without being told to guess.
const (
	SpeedTrendSteady = "steady"
	SpeedTrendRising = "rising"
	SpeedTrendEasing = "easing"
)

// DecisionInput is bounded, semantic context for one autonomous decision.
type DecisionInput struct {
	Style             string
	SegmentIndex      int
	RecentPatternIDs  []string
	SpeedMinPercent   int
	SpeedMaxPercent   int
	LastSay           string
	CurrentPatternID  motion.PatternID
	CurrentSpeed      int
	CurrentAreaFocus  *motion.AreaFocus
	CurrentDynamic    *motion.DynamicDefinition
	MotionMinSeconds  int
	MotionMaxSeconds  int
	MotionChangeLevel int
	// SessionSeconds, SecondsAtCurrentSpeed, and SpeedTrend are backend-computed
	// session facts. They are read-only input: the model cannot fabricate them,
	// they appear in traces, and they authorize nothing on their own. This is
	// deliberately not a model-maintained score — an accumulating counter that
	// feeds back into intensity is the hidden-escalation shape
	// docs/goals-and-guardrails.md rules out.
	SessionSeconds        int
	SecondsAtCurrentSpeed int
	SpeedTrend            string
	// SessionTracking reports whether the three fields above are meaningful. When
	// it is false the prompt omits them rather than sending zeros, because a model
	// cannot act on a field it never saw.
	SessionTracking bool
	// ArcEnabled and ArcPercent describe the visible session buildup. Absent from the
	// prompt entirely when disabled.
	ArcEnabled bool
	ArcPercent int
}

// Decision is one motion or speech curation outcome. Hold is scheduler-only:
// it must never produce an engine retarget.
type Decision struct {
	Segment Segment
	Pattern *motion.PatternDefinition
	Say     string
	Hold    bool
	Next    TimingPreference
	// Variability is how much the target should wander before the next boundary.
	Variability VariabilityPreference
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
	variability     VariabilityPreference
	decisionLatency time.Duration
}

// nextSegmentChoice picks the next segment for Freestyle or Autopilot.
func (m *Manager) nextSegmentChoice(ctx context.Context, mode string) segmentChoice {
	if mode != ModeAutopilot || m.options.Decide == nil {
		segment, scores := m.nextPlannedSegment()
		return segmentChoice{
			segment: segment, scores: scores, source: "planner",
			timing: TimingNormal, variability: VariabilityNormal,
		}
	}
	return m.runDecision(ctx, m.options.Decide, true)
}

// runDecision executes one model decision. Pattern-mode motion may fall back to
// the deterministic planner; Dynamic motion holds or waits because a catalog
// segment would violate the selected control mode. Speech decisions return
// their error so they can be postponed without disturbing motion.
func (m *Manager) runDecision(ctx context.Context, decide DecideFunc, fallback bool) segmentChoice {
	input := m.decisionInput()
	dynamicMode := m.options.MotionGenerationMode != nil &&
		m.options.MotionGenerationMode() == config.LLMMotionModeDynamic
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
		if dynamicMode {
			if segment, pattern, ok := m.heldSegment(); ok {
				return segmentChoice{
					segment: segment, pattern: pattern, source: "hold", note: err.Error(),
					timing: TimingNormal, variability: VariabilityNormal, decisionLatency: latency,
				}
			}
			return segmentChoice{source: "hold", note: err.Error(), timing: TimingNormal,
				variability: VariabilityNormal, decisionLatency: latency}
		}
		segment, scores := m.nextPlannedSegment()
		segment.DriftToSpeedPercent = 0
		return segmentChoice{
			segment: segment, scores: scores, source: "fallback", note: err.Error(),
			timing: TimingNormal, decisionLatency: latency,
		}
	}
	timing := normalizeTiming(decision.Next)
	variability := normalizeVariability(decision.Variability)
	if decision.Hold {
		if segment, pattern, ok := m.heldSegment(); ok {
			return segmentChoice{
				segment: segment, pattern: pattern, source: "hold",
				say: decision.Say, timing: timing, variability: variability,
				decisionLatency: latency,
			}
		}
		if !fallback || dynamicMode {
			return segmentChoice{
				source: "hold", say: decision.Say, timing: timing,
				variability: variability, decisionLatency: latency,
			}
		}
		segment, scores := m.nextPlannedSegment()
		segment.DriftToSpeedPercent = 0
		return segmentChoice{
			segment: segment, scores: scores, source: "fallback",
			note: "hold_without_segment", say: decision.Say, timing: timing,
			variability: variability, decisionLatency: latency,
		}
	}
	return segmentChoice{
		segment: NormalizeSegment(decision.Segment), pattern: decision.Pattern,
		source: "model", say: decision.Say, timing: timing,
		variability: variability, decisionLatency: latency,
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

// normalizeVariability keeps the manager defensive for planner/fallback and
// direct test callers. Model motion turns validate the required category before
// they reach this boundary.
func normalizeVariability(variability VariabilityPreference) VariabilityPreference {
	switch variability {
	case VariabilitySettled, VariabilityNormal, VariabilityRestless:
		return variability
	default:
		return VariabilityNormal
	}
}

// decisionInput snapshots the bounded model-visible context.
func (m *Manager) decisionInput() DecisionInput {
	settings := m.options.Settings()
	var current *motion.MotionTarget
	if engine := m.options.Current(); engine != nil {
		snapshot := engine.Snapshot()
		if (snapshot.Running || snapshot.Paused) && (snapshot.Target.PatternID != "" || snapshot.Target.Dynamic != nil) {
			copied := cloneTarget(snapshot.Target)
			current = &copied
		}
	}

	autopilot := m.options.AutopilotSettings()
	// Session elapsed comes from the engine's own run clock rather than a mode
	// counter, so a pause or a restart cannot inflate it.
	sessionSeconds := 0
	if engine := m.options.Current(); engine != nil {
		if running := engine.Snapshot().RunningMillis; running > 0 {
			sessionSeconds = int(running / 1000)
		}
	}

	now := m.options.Now()
	m.mu.Lock()
	input := DecisionInput{
		Style:            settings.Style,
		SegmentIndex:     m.segmentIdx,
		RecentPatternIDs: append([]string(nil), m.recentPatternIDs...),
		SpeedMinPercent:  settings.SpeedMinPercent,
		SpeedMaxPercent:  settings.SpeedMaxPercent,
		LastSay:          m.lastSay,
		SessionTracking:  autopilot.SessionTracking,
	}
	if autopilot.SessionTracking {
		input.SessionSeconds = sessionSeconds
		input.SpeedTrend = m.speedTrendLocked()
		if !m.speedChangedAt.IsZero() {
			if held := now.Sub(m.speedChangedAt); held > 0 {
				input.SecondsAtCurrentSpeed = int(held / time.Second)
			}
		}
		if autopilot.SessionArc {
			input.ArcEnabled = true
			input.ArcPercent = m.arcPercentLocked(now)
		}
	}
	if current == nil && m.segment.hasContent() {
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
		input.CurrentDynamic = cloneDynamicDefinition(current.Dynamic)
	}
	input.MotionMinSeconds, input.MotionMaxSeconds = autopilot.MotionWindow()
	input.MotionChangeLevel = autopilot.MotionChangeLevel
	return input
}

// speedTrendLocked reports the direction of the last speed change. Callers hold
// the lock.
func (m *Manager) speedTrendLocked() string {
	if m.previousSpeed <= 0 || m.segment.SpeedPercent <= 0 ||
		m.segment.SpeedPercent == m.previousSpeed {
		return SpeedTrendSteady
	}
	if m.segment.SpeedPercent > m.previousSpeed {
		return SpeedTrendRising
	}
	return SpeedTrendEasing
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
	if (target.PatternID == "" && target.Dynamic == nil) || target.SpeedPercent <= 0 {
		return Segment{}, nil, false
	}
	copied := cloneTarget(target)
	segment := NormalizeSegment(Segment{
		PatternID:      copied.PatternID,
		SpeedPercent:   copied.SpeedPercent,
		AreaFocus:      copied.AreaFocus,
		Dynamic:        copied.Dynamic,
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
	if choice.source == "hold" || !choice.segment.hasContent() {
		return
	}
	if choice.segment.PatternID != "" {
		m.recentPatternIDs = append(m.recentPatternIDs, string(choice.segment.PatternID))
		if len(m.recentPatternIDs) > 4 {
			m.recentPatternIDs = m.recentPatternIDs[len(m.recentPatternIDs)-4:]
		}
	}
}

func modeLabel(mode string) string {
	if mode == ModeAutopilot {
		return "Autopilot"
	}
	return "Freestyle"
}
