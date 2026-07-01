package motion

import (
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

const (
	minPeriodMillis = int64(550)
	maxPeriodMillis = int64(2500)
)

// MotionPlan is a repeatable semantic pattern sampled over stream time.
//
//revive:disable-next-line:exported -- Phase 6 explicitly names this contract.
type MotionPlan struct {
	ID             string       `json:"id"`
	Target         MotionTarget `json:"target"`
	PatternID      PatternID    `json:"pattern_id"`
	PeriodMillis   int64        `json:"period_ms"`
	HandoffMillis  int64        `json:"handoff_ms"`
	PhaseOffset    float64      `json:"phase_offset"`
	PhasePreserved bool         `json:"phase_preserved"`
	CreatedAt      string       `json:"created_at"`
}

// MotionSample is one transport-neutral semantic sample.
//
//revive:disable-next-line:exported -- keeps sampler state explicit with MotionPlan.
type MotionSample struct {
	PlanID          string    `json:"plan_id"`
	PatternID       PatternID `json:"pattern_id"`
	PositionPercent int       `json:"position_percent"`
	TimeMillis      int64     `json:"time_ms"`
	Phase           float64   `json:"phase"`
}

// NewMotionPlan returns a normalized plan for a target.
func NewMotionPlan(
	id string,
	target MotionTarget,
	settings config.MotionSettings,
	phaseOffset float64,
	handoffMillis int64,
	createdAt time.Time,
) MotionPlan {
	settings = normalizeMotionSettings(settings)
	target = NormalizeTarget(target, settings)
	if id == "" {
		id = fmt.Sprintf("%s-%d", target.PatternID, createdAt.UnixNano())
	}
	return MotionPlan{
		ID:            id,
		Target:        target,
		PatternID:     target.PatternID,
		PeriodMillis:  periodForSpeed(target.SpeedPercent),
		HandoffMillis: handoffMillis,
		PhaseOffset:   normalizePhase(phaseOffset),
		CreatedAt:     createdAt.UTC().Format(time.RFC3339Nano),
	}
}

// SampleAt samples the plan at the given stream-relative time.
func (p MotionPlan) SampleAt(streamMillis int64) MotionSample {
	phase := p.PhaseAt(streamMillis)
	value := samplePatternValue(p.PatternID, phase)
	position := applyTargetFocus(value, p.Target)
	return MotionSample{
		PlanID:          p.ID,
		PatternID:       p.PatternID,
		PositionPercent: clamp(int(math.Round(position)), 0, 100),
		TimeMillis:      streamMillis,
		Phase:           phase,
	}
}

// PhaseAt returns the semantic phase at the given stream-relative time.
func (p MotionPlan) PhaseAt(streamMillis int64) float64 {
	elapsed := streamMillis - p.HandoffMillis
	if elapsed < 0 {
		elapsed = 0
	}
	if p.PeriodMillis <= 0 {
		return normalizePhase(p.PhaseOffset)
	}
	return normalizePhase(p.PhaseOffset + float64(elapsed)/float64(p.PeriodMillis))
}

// Retarget returns a replacement plan, preserving phase for same-pattern updates.
func (p MotionPlan) Retarget(
	id string,
	target MotionTarget,
	settings config.MotionSettings,
	streamMillis int64,
	createdAt time.Time,
) MotionPlan {
	target = NormalizeTarget(target, normalizeMotionSettings(settings))
	phase := p.PhaseAt(streamMillis)
	preserved := p.PatternID == target.PatternID
	if !preserved {
		phase = chooseNearestPhase(target, settings, p.SampleAt(streamMillis).PositionPercent, p.DirectionAt(streamMillis))
	}
	next := NewMotionPlan(id, target, settings, phase, streamMillis, createdAt)
	next.PhasePreserved = preserved
	return next
}

// DirectionAt estimates current semantic travel direction at stream-relative time.
func (p MotionPlan) DirectionAt(streamMillis int64) int {
	const probeMillis = int64(25)
	before := p.SampleAt(streamMillis - probeMillis).PositionPercent
	after := p.SampleAt(streamMillis + probeMillis).PositionPercent
	switch {
	case after > before:
		return 1
	case after < before:
		return -1
	default:
		return 0
	}
}

func periodForSpeed(speedPercent int) int64 {
	speedPercent = clamp(speedPercent, 1, 100)
	period := maxPeriodMillis - int64(speedPercent*20)
	if period < minPeriodMillis {
		return minPeriodMillis
	}
	return period
}

func applyTargetFocus(value float64, target MotionTarget) float64 {
	minimum := 0.0
	maximum := 100.0
	if target.AreaFocus != nil {
		minimum = float64(target.AreaFocus.MinPercent)
		maximum = float64(target.AreaFocus.MaxPercent)
	}
	position := minimum + (maximum-minimum)*value
	if target.SoftAnchor != nil {
		weight := float64(target.SoftAnchor.WeightPercent) / 100.0
		position = position*(1-weight) + float64(target.SoftAnchor.PositionPercent)*weight
	}
	return position
}

func samplePatternValue(patternID PatternID, phase float64) float64 {
	knots, ok := fixedPatterns[patternID]
	if !ok {
		knots = fixedPatterns[PatternStroke]
	}
	phase = normalizePhase(phase)
	for index := 1; index < len(knots); index++ {
		if phase <= knots[index].phase {
			return interpolateKnot(knots[index-1], knots[index], phase)
		}
	}
	return knots[len(knots)-1].value
}

func chooseNearestPhase(target MotionTarget, settings config.MotionSettings, current int, currentDirection int) float64 {
	candidatePlan := NewMotionPlan("candidate", target, settings, 0, 0, time.Unix(0, 0))
	bestPhase := 0.0
	bestDistance := math.MaxFloat64
	for index := range 64 {
		phase := float64(index) / 64.0
		position := candidatePlan.SampleAt(int64(float64(candidatePlan.PeriodMillis) * phase)).PositionPercent
		distance := math.Abs(float64(position - current))
		candidateDirection := candidatePlan.DirectionAt(int64(float64(candidatePlan.PeriodMillis) * phase))
		score := distance
		if currentDirection != 0 && candidateDirection != 0 && candidateDirection != currentDirection {
			score += 8
		}
		if candidateDirection == 0 && distance > 2 {
			score += 4
		}
		if score < bestDistance {
			bestDistance = score
			bestPhase = phase
		}
	}
	return bestPhase
}

func interpolateKnot(left patternKnot, right patternKnot, phase float64) float64 {
	width := right.phase - left.phase
	if width <= 0 {
		return right.value
	}
	fraction := (phase - left.phase) / width
	return left.value + (right.value-left.value)*fraction
}

func normalizePhase(phase float64) float64 {
	phase = math.Mod(phase, 1)
	if phase < 0 {
		phase++
	}
	return phase
}

type patternKnot struct {
	phase float64
	value float64
}

var fixedPatterns = map[PatternID][]patternKnot{
	PatternStroke: {
		{phase: 0.00, value: 0.00},
		{phase: 0.50, value: 1.00},
		{phase: 1.00, value: 0.00},
	},
	PatternPulse: {
		{phase: 0.00, value: 0.15},
		{phase: 0.20, value: 1.00},
		{phase: 0.40, value: 0.25},
		{phase: 0.70, value: 0.85},
		{phase: 1.00, value: 0.15},
	},
	PatternTease: {
		{phase: 0.00, value: 0.25},
		{phase: 0.35, value: 0.45},
		{phase: 0.70, value: 1.00},
		{phase: 1.00, value: 0.25},
	},
}
