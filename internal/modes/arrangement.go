// Package modes implements autonomous motion behaviors (Freestyle, continuous
// chat) as clients of the motion engine. Modes emit bounded, named segments
// that deterministic code applies through the engine's retarget API — never a
// parallel sampler, never transport commands, and never per-turn low-level
// stream replacement (docs/motion-retargeting.md, "Route Policy Learned On
// Hardware").
package modes

import (
	"errors"
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	minSegmentMillis = int64(4_000)
	maxSegmentMillis = int64(120_000)
	maxSegments      = 8
)

// Segment is one bounded step of a motion arrangement: a named pattern at a
// semantic speed, optionally focused and optionally drifting to a second
// speed mid-segment. A segment always runs long enough to establish a feel.
type Segment struct {
	PatternID           motion.PatternID  `json:"pattern_id"`
	SpeedPercent        int               `json:"speed_percent"`
	DriftToSpeedPercent int               `json:"drift_to_speed_percent,omitempty"`
	AreaFocus           *motion.AreaFocus `json:"area_focus,omitempty"`
	DurationMillis      int64             `json:"duration_ms"`
}

// Arrangement is a bounded, inspectable sequence of segments. Planners and
// (later) LLM curation produce arrangements; deterministic code compiles them
// into engine retargets with explicit transition rules.
type Arrangement struct {
	Label    string    `json:"label"`
	Segments []Segment `json:"segments"`
}

// NormalizeSegment clamps one segment into the bounded contract.
func NormalizeSegment(segment Segment) Segment {
	if segment.PatternID == "" {
		segment.PatternID = motion.PatternStroke
	}
	segment.SpeedPercent = clampInt(segment.SpeedPercent, 1, 100)
	if segment.DriftToSpeedPercent != 0 {
		segment.DriftToSpeedPercent = clampInt(segment.DriftToSpeedPercent, 1, 100)
	}
	segment.DurationMillis = clampInt64(segment.DurationMillis, minSegmentMillis, maxSegmentMillis)
	return segment
}

// NormalizeArrangement validates the bounded contract for a whole arrangement.
func NormalizeArrangement(arrangement Arrangement) (Arrangement, error) {
	if len(arrangement.Segments) == 0 {
		return Arrangement{}, errors.New("arrangement requires at least one segment")
	}
	if len(arrangement.Segments) > maxSegments {
		return Arrangement{}, fmt.Errorf("arrangement is limited to %d segments", maxSegments)
	}
	for index, segment := range arrangement.Segments {
		arrangement.Segments[index] = NormalizeSegment(segment)
	}
	return arrangement, nil
}

// Target converts a segment into the engine's semantic target. The engine
// clamps speed into the user's limits and owns every transport decision.
func (s Segment) Target(label string, source string) motion.MotionTarget {
	return motion.MotionTarget{
		Label:        label,
		Source:       source,
		PatternID:    s.PatternID,
		SpeedPercent: s.SpeedPercent,
		AreaFocus:    s.AreaFocus,
	}
}

// DriftTarget is the optional mid-segment same-pattern speed nudge; phase is
// preserved because the pattern does not change.
func (s Segment) DriftTarget(label string, source string) (motion.MotionTarget, bool) {
	if s.DriftToSpeedPercent == 0 || s.DriftToSpeedPercent == s.SpeedPercent {
		return motion.MotionTarget{}, false
	}
	target := s.Target(label, source)
	target.SpeedPercent = s.DriftToSpeedPercent
	return target, true
}

func clampInt(value int, minimum int, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func clampInt64(value int64, minimum int64, maximum int64) int64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
