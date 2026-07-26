package modes

import (
	"context"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Autopilot is Freestyle's loop with the segment choice delegated to an
// injected LLM curation step. The model curates *what* plays next (an enabled
// pattern, an intensity, and an optional assistant line); deterministic code owns
// *how long* it plays, every clamp, and the whole engine/transport path. Any
// decision failure falls back to the deterministic planner with a visible
// trace — motion never stalls and never stops on a model failure, and only
// the user ever stops the device.

// decisionTimeout bounds one LLM curation call. The mode loop context still
// cancels it immediately on Stop or mode switch.
const decisionTimeout = 25 * time.Second

// DecisionInput is the bounded, model-visible context for one curation step.
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

// Decision is one curation outcome. Hold keeps the current segment's pattern
// and speed for another deterministic duration (the model chose to talk or
// leave motion alone). A model-requested stop is mapped to Hold by the caller:
// stopping the device belongs to the user.
type Decision struct {
	Segment Segment
	// Pattern optionally carries the resolved enabled library definition for
	// Segment.PatternID so the engine can play library content.
	Pattern *motion.PatternDefinition
	Say     string
	Hold    bool
}

// DecideFunc produces one bounded curation decision.
type DecideFunc func(ctx context.Context, input DecisionInput) (Decision, error)

// segmentChoice is what one autonomous tick actually applies.
type segmentChoice struct {
	segment Segment
	pattern *motion.PatternDefinition
	scores  []diagnostics.PlannerScore
	say     string
	source  string // "planner", "model", "fallback", "hold"
	note    string
}

// nextSegmentChoice picks the next segment for an autonomous mode. Freestyle
// is purely deterministic; Autopilot asks the injected decision step and
// falls back to the deterministic planner on any failure.
func (m *Manager) nextSegmentChoice(ctx context.Context, mode string) segmentChoice {
	if mode != ModeAutopilot || m.options.Decide == nil {
		segment, scores := m.nextPlannedSegment()
		return segmentChoice{segment: segment, scores: scores, source: "planner"}
	}

	input := m.decisionInput()
	decideCtx, cancel := context.WithTimeout(ctx, decisionTimeout)
	defer cancel()
	decision, err := m.options.Decide(decideCtx, input)
	if err != nil {
		segment, scores := m.nextPlannedSegment()
		return segmentChoice{segment: segment, scores: scores, source: "fallback", note: err.Error()}
	}
	if decision.Hold {
		if segment, pattern, ok := m.heldSegment(); ok {
			return segmentChoice{segment: segment, pattern: pattern, say: decision.Say, source: "hold"}
		}
		segment, scores := m.nextPlannedSegment()
		return segmentChoice{segment: segment, scores: scores, say: decision.Say, source: "fallback", note: "hold_without_segment"}
	}

	segment := NormalizeSegment(decision.Segment)
	if decision.Segment.DurationMillis == 0 {
		// The model curates what plays; deterministic style bounds decide how
		// long it plays so one decision can never pin an endless segment.
		segment.DurationMillis = m.plannedDurationMillis()
	}
	return segmentChoice{segment: segment, pattern: decision.Pattern, say: decision.Say, source: "model"}
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

// heldSegment re-arms the current segment (same pattern, same speed) for a
// fresh deterministic duration.
func (m *Manager) heldSegment() (Segment, *motion.PatternDefinition, bool) {
	duration := m.plannedDurationMillis()
	if engine := m.options.Current(); engine != nil {
		snapshot := engine.Snapshot()
		if snapshot.Running || snapshot.Paused {
			if segment, pattern, ok := segmentFromMotionTarget(snapshot.Target, duration); ok {
				return segment, pattern, true
			}
		}
	}

	m.mu.Lock()
	target := m.segment.Target(modeLabel(m.mode), m.mode)
	target.Pattern = m.pattern
	target = cloneTarget(target)
	m.mu.Unlock()
	return segmentFromMotionTarget(target, duration)
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

func (m *Manager) currentStyleProfile() styleProfile {
	settings := m.options.Settings()
	profile, ok := styleProfiles[settings.Style]
	if !ok {
		profile = styleProfiles[config.MotionStyleBalanced]
	}
	return profile
}

// plannedDurationMillis borrows the deterministic style duration bounds.
func (m *Manager) plannedDurationMillis() int64 {
	profile := m.currentStyleProfile()
	m.mu.Lock()
	planner := m.planner
	m.mu.Unlock()
	if planner == nil {
		return profile.minDurationMillis
	}
	return planner.durationFor(profile)
}

// rememberChoice records segment provenance for status and future decisions.
func (m *Manager) rememberChoice(mode string, choice segmentChoice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mode != mode {
		return
	}
	if mode == ModeAutopilot {
		m.decisionSource = choice.source
		if choice.say != "" {
			m.lastSay = choice.say
		}
	}
	m.recentPatternIDs = append(m.recentPatternIDs, string(choice.segment.PatternID))
	if len(m.recentPatternIDs) > 4 {
		m.recentPatternIDs = m.recentPatternIDs[len(m.recentPatternIDs)-4:]
	}
}

// modeLabel names targets for the UI/status ("Label") per autonomous mode.
func modeLabel(mode string) string {
	if mode == ModeAutopilot {
		return "Autopilot"
	}
	return "Freestyle"
}
