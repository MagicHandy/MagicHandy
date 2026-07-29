package motion

import (
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// At 100%, media may traverse the full semantic stroke three times per
// second. Lower speed limits cap only segments that exceed that rate; they do
// not contract every authored excursion and turn ordinary slow strokes into
// sub-minimum-speed moves with a dwell at each reversal.
const mediaFullSpeedRatePercentPerSecond = 300.0

// MotionPlan is repeatable or finite semantic content sampled over stream time.
//
//revive:disable-next-line:exported -- Phase 6 explicitly names this contract.
type MotionPlan struct {
	ID             string       `json:"id"`
	Target         MotionTarget `json:"target"`
	PatternID      PatternID    `json:"pattern_id,omitempty"`
	ProgramID      string       `json:"program_id,omitempty"`
	MediaID        string       `json:"media_id,omitempty"`
	PeriodMillis   int64        `json:"period_ms"`
	HandoffMillis  int64        `json:"handoff_ms"`
	PhaseOffset    float64      `json:"phase_offset"`
	PhasePreserved bool         `json:"phase_preserved"`
	Loop           bool         `json:"loop"`
	CreatedAt      string       `json:"created_at"`

	curve Curve
	focus focusProjection
}

// MotionSample is one transport-neutral semantic sample.
//
//revive:disable-next-line:exported -- keeps sampler state explicit with MotionPlan.
type MotionSample struct {
	PlanID          string    `json:"plan_id"`
	PatternID       PatternID `json:"pattern_id,omitempty"`
	ProgramID       string    `json:"program_id,omitempty"`
	MediaID         string    `json:"media_id,omitempty"`
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
	target, content := resolveTargetContent(target)
	if id == "" {
		id = fmt.Sprintf("%s-%d", motionContentID(target), createdAt.UnixNano())
	}
	focus := newFocusProjection(target, content)
	periodMillis := periodForContent(content.duration, target.SpeedPercent, content.loop, patternKind(target))
	if target.Media != nil {
		periodMillis = content.duration
	} else {
		periodMillis = focusedLoopPeriod(periodMillis, content.duration, content.loop, patternKind(target), focus)
	}
	return MotionPlan{
		ID:            id,
		Target:        target,
		PatternID:     target.PatternID,
		ProgramID:     target.ProgramID,
		MediaID:       target.MediaID,
		PeriodMillis:  periodMillis,
		HandoffMillis: handoffMillis,
		PhaseOffset:   phaseForContent(phaseOffset, content.loop),
		Loop:          content.loop,
		CreatedAt:     createdAt.UTC().Format(time.RFC3339Nano),
		curve:         content.buildCurve(content.playbackScale(focus, periodMillis)),
		focus:         focus,
	}
}

// SampleAt samples the plan at the given stream-relative time.
func (p MotionPlan) SampleAt(streamMillis int64) MotionSample {
	phase := p.PhaseAt(streamMillis)
	curveMillis := int64(math.Round(phase * float64(p.curve.duration)))
	position := p.focus.apply(p.curve.Sample(curveMillis))
	return MotionSample{
		PlanID:          p.ID,
		PatternID:       p.PatternID,
		ProgramID:       p.ProgramID,
		MediaID:         p.MediaID,
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
	return p.retargetFromState(
		id, target, settings, streamMillis,
		p.SampleAt(streamMillis).PositionPercent,
		p.DirectionAt(streamMillis),
		createdAt,
	)
}

func (p MotionPlan) retargetFromState(
	id string,
	target MotionTarget,
	settings config.MotionSettings,
	streamMillis int64,
	currentPosition float64,
	currentDirection int,
	createdAt time.Time,
) MotionPlan {
	target = NormalizeTarget(target, normalizeMotionSettings(settings))
	phase := p.PhaseAt(streamMillis)
	preserved := motionContentID(p.Target) == motionContentID(target)
	if !preserved {
		phase = chooseNearestPhase(target, settings, currentPosition, currentDirection)
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

// resolvedContent is validated motion content whose curve is not built yet.
// The reversal ramp depends on the speed and focus the content will be played
// at, and the period is only known after the content duration is, so the curve
// is the last thing the plan constructs.
type resolvedContent struct {
	points        []CurvePoint
	duration      int64
	loop          bool
	linear        bool
	maximumPoints int
}

func (c resolvedContent) buildCurve(scale playbackScale) Curve {
	curve, _ := newCurve(c.points, c.duration, c.loop, c.linear, c.maximumPoints, scale)
	return curve
}

func (c resolvedContent) playbackScale(focus focusProjection, periodMillis int64) playbackScale {
	scale := neutralPlaybackScale()
	if c.duration > 0 && periodMillis > 0 {
		scale.timeFactor = float64(periodMillis) / float64(c.duration)
	}
	scale.amplitudeFactor = focus.gain()
	return scale
}

func resolveTargetContent(target MotionTarget) (MotionTarget, resolvedContent) {
	if target.Media != nil {
		if definition, err := NormalizeMediaTimelineDefinition(*target.Media); err == nil {
			if target.MediaSpeedLimitEnabled {
				definition.Points = limitMediaTimelineRate(definition.Points, target.SpeedPercent)
			}
			target.Media = &definition
			target.MediaID = definition.ID
			target.PatternID = ""
			target.PatternName = ""
			target.ProgramID = ""
			return target, resolvedContent{
				points: definition.Points, duration: definition.DurationMillis,
				linear: true, maximumPoints: MaximumMediaTimelinePoints,
			}
		}
		target.Media = nil
		target.MediaID = ""
	}
	if target.Program != nil {
		if definition, err := NormalizeProgramDefinition(*target.Program); err == nil {
			target.Program = &definition
			target.ProgramID = definition.ID
			target.PatternID = ""
			target.PatternName = ""
			target.MediaID = ""
			return target, resolvedContent{
				points: definition.Points, duration: definition.DurationMillis,
				maximumPoints: maximumCurvePoints,
			}
		}
		target.Program = nil
		target.ProgramID = ""
	}
	if target.Pattern != nil {
		if definition, err := NormalizePatternDefinition(*target.Pattern); err == nil {
			target.Pattern = &definition
			target.PatternID = definition.ID
			target.PatternName = definition.Name
			target.ProgramID = ""
			target.MediaID = ""
			return target, patternContent(definition)
		}
	}
	definition, ok := BuiltinPatternDefinition(target.PatternID)
	if !ok {
		definition, _ = BuiltinPatternDefinition(PatternStroke)
		target.PatternID = PatternStroke
	}
	target.Pattern = &definition
	target.PatternName = definition.Name
	target.ProgramID = ""
	target.MediaID = ""
	return target, patternContent(definition)
}

func patternContent(definition PatternDefinition) resolvedContent {
	return resolvedContent{
		points: definition.Points, duration: definition.CycleMillis,
		loop: true, maximumPoints: maximumCurvePoints,
	}
}

// limitMediaTimelineRate keeps the video clock and every point timestamp
// unchanged while bounding only physically over-fast travel. This is a
// conventional slew limiter: each accepted point becomes the anchor for the
// next segment, so the output cannot hide an unsafe jump after one clipped
// segment. Ordinary authored travel below the selected limit is unchanged.
func limitMediaTimelineRate(points []CurvePoint, speedPercent int) []CurvePoint {
	if len(points) < 2 {
		return append([]CurvePoint(nil), points...)
	}
	maximumRate := mediaFullSpeedRatePercentPerSecond * float64(clamp(speedPercent, 1, 100)) / 100
	limited := append([]CurvePoint(nil), points...)
	for index := 1; index < len(limited); index++ {
		elapsedMillis := limited[index].TimeMillis - limited[index-1].TimeMillis
		maximumDelta := maximumRate * float64(elapsedMillis) / 1000
		minimum := limited[index-1].PositionPercent - maximumDelta
		maximum := limited[index-1].PositionPercent + maximumDelta
		limited[index].PositionPercent = math.Max(
			minimum,
			math.Min(maximum, limited[index].PositionPercent),
		)
	}
	return limited
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

// focusedLoopPeriod keeps a narrowed loop from also becoming proportionally
// slower. The requested period contracts with travel, while the routine floor
// contracts only by sqrt(gain): acceleration scales with distance/time^2, so
// this preserves the catalog acceleration budget when a high requested speed
// cannot preserve both travel rate and acceleration.
func focusedLoopPeriod(period, authoredPeriod int64, loop bool, kind string, focus focusProjection) int64 {
	gain := focus.gain()
	if !loop || gain <= 0 || gain >= 1 {
		return period
	}
	adjusted := int64(math.Round(float64(period) * gain))
	baseline := max(authoredPeriod, int64(minimumBurstCycleMillis))
	minimum := int64(minimumBurstCycleMillis)
	if kind != PatternKindBurst {
		baseline = max(baseline, RoutineCycleFloorMillis)
		minimum = int64(math.Ceil(float64(baseline) * math.Sqrt(gain)))
	} else {
		minimum = max(minimum, int64(math.Ceil(float64(baseline)*math.Sqrt(gain))))
	}
	return max(adjusted, minimum)
}

func patternKind(target MotionTarget) string {
	if target.Pattern != nil {
		return target.Pattern.Kind
	}
	return PatternKindRoutine
}

func motionContentID(target MotionTarget) string {
	if target.MediaID != "" {
		return "media:" + target.MediaID
	}
	if target.ProgramID != "" {
		return "program:" + target.ProgramID
	}
	return "pattern:" + string(target.PatternID)
}

// minimumFocusSourceSpan is the smallest authored span worth re-expanding.
// Below it the shape is closer to noise than to a stroke, and stretching it to
// fill a window would amplify that noise (docs/motion-pathway-review, import
// noise amplification).
const minimumFocusSourceSpan = 5.0

// focusProjection maps an authored position onto the region motion is confined
// to. Confining a loop pattern re-expands its own span to fill the window, so
// asking for a smaller region changes where the stroke happens without also
// making the shape too subtle to feel. Finite programs and clock-locked media
// keep authored amplitude.
type focusProjection struct {
	sourceMin  float64
	sourceSpan float64
	targetMin  float64
	targetSpan float64
	anchor     *SoftAnchor
}

func newFocusProjection(
	target MotionTarget,
	content resolvedContent,
) focusProjection {
	projection := focusProjection{
		sourceMin: 0, sourceSpan: 100,
		targetMin: 0, targetSpan: 100,
		anchor: target.SoftAnchor,
	}
	focus := effectiveAreaFocus(target)
	if focus == nil {
		return projection
	}
	projection.targetMin = float64(focus.MinPercent)
	projection.targetSpan = float64(focus.MaxPercent) - projection.targetMin
	if !content.loop || len(content.points) == 0 {
		return projection
	}
	minimum, maximum := curvePointBounds(content.points)
	if maximum-minimum < minimumFocusSourceSpan {
		return projection
	}
	projection.sourceMin = minimum
	projection.sourceSpan = maximum - minimum
	return projection
}

// gain reports played percent per authored percent, which is what compresses
// the reversal acceleration budget.
func (f focusProjection) gain() float64 {
	if f.sourceSpan <= 0 {
		return 1
	}
	return f.targetSpan / f.sourceSpan
}

func (f focusProjection) apply(percent float64) float64 {
	position := percent
	if f.sourceSpan > 0 {
		position = f.targetMin + f.targetSpan*(percent-f.sourceMin)/f.sourceSpan
	}
	if f.anchor != nil {
		weight := float64(f.anchor.WeightPercent) / 100.0
		position = position*(1-weight) + float64(f.anchor.PositionPercent)*weight
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
