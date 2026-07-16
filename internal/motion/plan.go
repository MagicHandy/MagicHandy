package motion

import (
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// MotionPlan is repeatable or finite semantic content sampled over stream time.
//
//revive:disable-next-line:exported -- Phase 6 explicitly names this contract.
type MotionPlan struct {
	ID             string       `json:"id"`
	Target         MotionTarget `json:"target"`
	PatternID      PatternID    `json:"pattern_id,omitempty"`
	ProgramID      string       `json:"program_id,omitempty"`
	PeriodMillis   int64        `json:"period_ms"`
	HandoffMillis  int64        `json:"handoff_ms"`
	PhaseOffset    float64      `json:"phase_offset"`
	PhasePreserved bool         `json:"phase_preserved"`
	Loop           bool         `json:"loop"`
	CreatedAt      string       `json:"created_at"`

	curve Curve
}

// MotionSample is one transport-neutral semantic sample.
//
//revive:disable-next-line:exported -- keeps sampler state explicit with MotionPlan.
type MotionSample struct {
	PlanID          string    `json:"plan_id"`
	PatternID       PatternID `json:"pattern_id,omitempty"`
	ProgramID       string    `json:"program_id,omitempty"`
	PositionPercent float64   `json:"position_percent"`
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
	target, curve, loop, baseDuration := resolveTargetCurve(target)
	if id == "" {
		id = fmt.Sprintf("%s-%d", motionContentID(target), createdAt.UnixNano())
	}
	return MotionPlan{
		ID:            id,
		Target:        target,
		PatternID:     target.PatternID,
		ProgramID:     target.ProgramID,
		PeriodMillis:  periodForContent(baseDuration, target.SpeedPercent, loop, patternKind(target)),
		HandoffMillis: handoffMillis,
		PhaseOffset:   phaseForContent(phaseOffset, loop),
		Loop:          loop,
		CreatedAt:     createdAt.UTC().Format(time.RFC3339Nano),
		curve:         curve,
	}
}

// SampleAt samples the plan at the given stream-relative time.
func (p MotionPlan) SampleAt(streamMillis int64) MotionSample {
	phase := p.PhaseAt(streamMillis)
	curveMillis := int64(math.Round(phase * float64(p.curve.duration)))
	value := p.curve.Sample(curveMillis)
	position := applyTargetFocus(value/100, p.Target)
	return MotionSample{
		PlanID:          p.ID,
		PatternID:       p.PatternID,
		ProgramID:       p.ProgramID,
		PositionPercent: math.Max(0, math.Min(100, position)),
		TimeMillis:      streamMillis,
		Phase:           phase,
	}
}

// PhaseAt returns semantic phase at the given stream-relative time.
func (p MotionPlan) PhaseAt(streamMillis int64) float64 {
	elapsed := streamMillis - p.HandoffMillis
	if elapsed < 0 {
		elapsed = 0
	}
	if p.PeriodMillis <= 0 {
		return phaseForContent(p.PhaseOffset, p.Loop)
	}
	phase := p.PhaseOffset + float64(elapsed)/float64(p.PeriodMillis)
	return phaseForContent(phase, p.Loop)
}

// CompleteAt reports whether finite program content reached its final point.
func (p MotionPlan) CompleteAt(streamMillis int64) bool {
	if p.Loop || p.PeriodMillis <= 0 {
		return false
	}
	elapsed := max(int64(0), streamMillis-p.HandoffMillis)
	return p.PhaseOffset+float64(elapsed)/float64(p.PeriodMillis) >= 1
}

// Retarget returns a replacement plan, preserving phase for the same content.
func (p MotionPlan) Retarget(
	id string,
	target MotionTarget,
	settings config.MotionSettings,
	streamMillis int64,
	createdAt time.Time,
) MotionPlan {
	target = NormalizeTarget(target, normalizeMotionSettings(settings))
	phase := p.PhaseAt(streamMillis)
	preserved := motionContentID(p.Target) == motionContentID(target)
	if !preserved {
		phase = chooseNearestPhase(target, settings, p.SampleAt(streamMillis).PositionPercent, p.DirectionAt(streamMillis))
	}
	next := NewMotionPlan(id, target, settings, phase, streamMillis, createdAt)
	next.PhasePreserved = preserved
	return next
}

// DirectionAt estimates current semantic travel direction.
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

func resolveTargetCurve(target MotionTarget) (MotionTarget, Curve, bool, int64) {
	if target.Program != nil {
		if definition, err := NormalizeProgramDefinition(*target.Program); err == nil {
			curve, _ := NewCurve(definition.Points, definition.DurationMillis, false)
			target.Program = &definition
			target.ProgramID = definition.ID
			target.PatternID = ""
			return target, curve, false, definition.DurationMillis
		}
		target.Program = nil
		target.ProgramID = ""
	}
	if target.Pattern != nil {
		if definition, err := NormalizePatternDefinition(*target.Pattern); err == nil {
			curve, _ := NewCurve(definition.Points, definition.CycleMillis, true)
			target.Pattern = &definition
			target.PatternID = definition.ID
			target.ProgramID = ""
			return target, curve, true, definition.CycleMillis
		}
	}
	definition, ok := BuiltinPatternDefinition(target.PatternID)
	if !ok {
		definition, _ = BuiltinPatternDefinition(PatternStroke)
		target.PatternID = PatternStroke
	}
	curve, _ := NewCurve(definition.Points, definition.CycleMillis, true)
	target.Pattern = &definition
	target.ProgramID = ""
	return target, curve, true, definition.CycleMillis
}

func periodForContent(baseDuration int64, speedPercent int, loop bool, kind string) int64 {
	speedPercent = clamp(speedPercent, 1, 100)
	factor := 2 - float64(speedPercent)/100
	period := int64(math.Round(float64(baseDuration) * factor))
	if loop && kind != PatternKindBurst && period < RoutineCycleFloorMillis {
		return RoutineCycleFloorMillis
	}
	if period < minimumBurstCycleMillis {
		return minimumBurstCycleMillis
	}
	return period
}

func patternKind(target MotionTarget) string {
	if target.Pattern != nil {
		return target.Pattern.Kind
	}
	return PatternKindRoutine
}

func motionContentID(target MotionTarget) string {
	if target.ProgramID != "" {
		return "program:" + target.ProgramID
	}
	return "pattern:" + string(target.PatternID)
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

func chooseNearestPhase(target MotionTarget, settings config.MotionSettings, current float64, currentDirection int) float64 {
	candidatePlan := NewMotionPlan("candidate", target, settings, 0, 0, time.Unix(0, 0))
	bestPhase := 0.0
	bestDistance := math.MaxFloat64
	lastIndex := 63
	if !candidatePlan.Loop {
		lastIndex = 64
	}
	for index := range lastIndex + 1 {
		phase := float64(index) / 64
		position := candidatePlan.SampleAt(int64(float64(candidatePlan.PeriodMillis) * phase)).PositionPercent
		distance := math.Abs(position - current)
		candidateDirection := candidatePlan.DirectionAt(int64(float64(candidatePlan.PeriodMillis) * phase))
		score := handoffScore(distance, currentDirection, candidateDirection)
		if score < bestDistance {
			bestDistance = score
			bestPhase = phase
		}
	}
	return bestPhase
}

func handoffScore(distance float64, currentDirection, candidateDirection int) float64 {
	score := distance
	if currentDirection != 0 && candidateDirection != 0 && candidateDirection != currentDirection {
		score += 8
	}
	if candidateDirection == 0 && distance > 2 {
		score += 4
	}
	return score
}

func phaseForContent(phase float64, loop bool) float64 {
	if loop {
		return normalizePhase(phase)
	}
	return math.Max(0, math.Min(1, phase))
}

func normalizePhase(phase float64) float64 {
	phase = math.Mod(phase, 1)
	if phase < 0 {
		phase++
	}
	return phase
}
