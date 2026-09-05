package motion

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// maximumSupportedReferenceTravelRatePercentPerSecond is the fastest semantic
// rate among the documented, non-overclocked Handy profiles. Dynamic uses it
// to keep a varied phrase long enough under every supported calibration.
const maximumSupportedReferenceTravelRatePercentPerSecond = 400.0 / 110.0 * 100.0

type handySpeedProfile struct {
	strokeMillimeters  float64
	minimumMMPerSecond float64
	maximumMMPerSecond float64
}

// MotionPlan is repeatable or finite semantic content sampled over stream time.
//
//revive:disable-next-line:exported -- Phase 6 explicitly names this contract.
type MotionPlan struct {
	ID             string            `json:"id"`
	Target         MotionTarget      `json:"target"`
	PatternID      PatternID         `json:"pattern_id,omitempty"`
	ProgramID      string            `json:"program_id,omitempty"`
	MediaID        string            `json:"media_id,omitempty"`
	PeriodMillis   int64             `json:"period_ms"`
	HandoffMillis  int64             `json:"handoff_ms"`
	PhaseOffset    float64           `json:"phase_offset"`
	PhasePreserved bool              `json:"phase_preserved"`
	Loop           bool              `json:"loop"`
	CreatedAt      string            `json:"created_at"`
	Perceptual     PerceptualSummary `json:"perceptual"`

	curve Curve
	focus focusProjection
	// compileErr is never serialized. The engine rejects a plan carrying it;
	// the safe stationary curve only keeps diagnostic/preview sampling total so
	// malformed semantic content cannot panic the whole process.
	compileErr error
	// timingModel is retained only so a Dynamic retarget can tell whether the
	// selected device calibration changed its locally fitted curve clock.
	timingModel string
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
	if target.prepared != nil && target.prepared.libraryID == "" {
		return preparedPlan(id, target, settings, phaseOffset, handoffMillis, createdAt)
	}
	target, content := resolveTargetContent(target, settings.HandyModel)
	preparedTarget, prepareErr := prepareContinuousRecipe(target, settings)
	if prepareErr == nil {
		target = preparedTarget
	}
	if target.prepared != nil {
		plan := preparedPlan(id, target, settings, phaseOffset, handoffMillis, createdAt)
		plan.compileErr = prepareErr
		return plan
	}
	timingErr := error(nil)
	if prepareErr != nil {
		timingErr = prepareErr
	}
	if target.Dynamic != nil {
		content, timingErr = retimeDynamicContent(content, target.SpeedPercent, settings.HandyModel)
	}
	if id == "" {
		id = fmt.Sprintf("%s-%d", motionContentID(target), createdAt.UnixNano())
	}
	compileErr := timingErr
	if compileErr == nil {
		compileErr = content.validate()
	}
	focus := focusProjection{sourceMin: 0, sourceSpan: 100, targetMin: 0, targetSpan: 100}
	if compileErr == nil {
		focus = newFocusProjection(target, content)
	}
	periodMillis := int64(minimumBurstCycleMillis)
	var curve Curve
	if compileErr == nil {
		if content.timingResolved {
			periodMillis = content.duration
		} else {
			periodMillis = periodForContent(
				content.points,
				content.duration,
				target.SpeedPercent,
				content.loop,
				settings.HandyModel,
			)
		}
		if target.Media != nil {
			periodMillis = content.duration
		} else {
			minimumPeriod := int64(minimumBurstCycleMillis)
			if content.timingResolved {
				// Creative already has a locally fitted wall clock. Keep only the
				// real reversal/acceleration/jerk envelope here; the 500 ms catalog
				// burst floor is not a device constraint.
				minimumPeriod = 1
			}
			periodMillis = focusedLoopPeriodWithMinimum(
				periodMillis, content.points, content.duration, content.loop, focus,
				content.reversalProfile, minimumPeriod,
			)
		}
		curve, compileErr = content.buildCurve(content.playbackScale(focus, periodMillis))
	}
	if compileErr != nil {
		compileErr = fmt.Errorf("compile motion plan: %w", compileErr)
		periodMillis = minimumBurstCycleMillis
		curve = content.stationaryFallbackCurve()
	}
	perceptual := PerceptualSummary{}
	if compileErr == nil && target.Dynamic != nil {
		perceptual = summarizeMotionPlan(target, curve, focus, periodMillis, settings.HandyModel)
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
		Perceptual:    perceptual,
		curve:         curve,
		focus:         focus,
		compileErr:    compileErr,
		timingModel:   settings.HandyModel,
	}
}

func (p MotionPlan) compilationError() error {
	return p.compileErr
}

// SampleAt samples the plan at the given stream-relative time.
func (p MotionPlan) SampleAt(streamMillis int64) MotionSample {
	phase := p.PhaseAt(streamMillis)
	// Preserve fractional authored time. Rounding here used to turn a single
	// authored millisecond into a long exact plateau when a curve was played
	// slowly, even though the semantic curve itself remained continuous.
	curveMillis := phase * float64(p.curve.duration)
	position := p.focus.apply(p.curve.sampleFloat(curveMillis))
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
		p.VelocityAt(streamMillis),
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
	currentVelocity float64,
	createdAt time.Time,
) MotionPlan {
	target = NormalizeTarget(target, normalizeMotionSettings(settings))
	phase := p.PhaseAt(streamMillis)
	preserved := motionContentID(p.Target) == motionContentID(target)
	if preserved && target.Dynamic != nil &&
		(p.Target.SpeedPercent != target.SpeedPercent || p.timingModel != settings.HandyModel) {
		// Creative fits unsafe intervals locally, so changing its requested
		// carriage velocity or device profile changes the normalized timing of
		// some legs. Match the current position and direction on that new clock
		// rather than pretending the old phase has the same physical meaning.
		preserved = false
	}
	if !preserved {
		phase = chooseNearestPhase(target, settings, currentPosition, currentDirection, currentVelocity)
	}
	next := NewMotionPlan(id, target, settings, phase, streamMillis, createdAt)
	next.PhasePreserved = preserved
	return next
}

// VelocityAt estimates played semantic velocity in percent per second. It is
// used only to choose a smooth retarget phase; transports still receive the
// normal sampled plan.
func (p MotionPlan) VelocityAt(streamMillis int64) float64 {
	const probeMillis = int64(25)
	before := p.SampleAt(streamMillis - probeMillis).PositionPercent
	after := p.SampleAt(streamMillis + probeMillis).PositionPercent
	return (after - before) * 1000 / float64(probeMillis*2)
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
	points          []CurvePoint
	duration        int64
	loop            bool
	linear          bool
	maximumPoints   int
	reversalProfile curveReversalProfile
	timingResolved  bool
	preserveRhythm  bool
}

func (c resolvedContent) validate() error {
	if len(c.points) < 2 {
		return errors.New("a motion curve requires at least two points")
	}
	maximumPoints := c.maximumPoints
	if maximumPoints <= 0 {
		maximumPoints = maximumCurvePoints
	}
	if len(c.points) > maximumPoints {
		return fmt.Errorf("a motion curve supports at most %d points", maximumPoints)
	}
	return validateCurvePoints(c.points, c.duration)
}

func (c resolvedContent) buildCurve(scale playbackScale) (Curve, error) {
	maximumPoints := c.maximumPoints
	if maximumPoints <= 0 {
		maximumPoints = maximumCurvePoints
	}
	return newCurveWithReversalProfile(
		c.points, c.duration, c.loop, c.linear, maximumPoints, scale,
		c.reversalProfile,
	)
}

func (c resolvedContent) stationaryFallbackCurve() Curve {
	position := 50.0
	for _, point := range c.points {
		if !math.IsNaN(point.PositionPercent) && !math.IsInf(point.PositionPercent, 0) {
			position = math.Max(0, math.Min(100, point.PositionPercent))
			break
		}
	}
	points := []CurvePoint{
		{TimeMillis: 0, PositionPercent: position},
		{TimeMillis: minimumBurstCycleMillis, PositionPercent: position},
	}
	return Curve{
		points: points, authoredKnots: append([]CurvePoint(nil), points...),
		duration: minimumBurstCycleMillis, linear: true,
		minPosition: position, maxPosition: position,
	}
}

func (c resolvedContent) playbackScale(focus focusProjection, periodMillis int64) playbackScale {
	scale := neutralPlaybackScale()
	scale.maxAccelerationPercentSecond2 = runtimeMaxAccelerationPercentPerSecond2
	if c.duration > 0 && periodMillis > 0 {
		scale.timeFactor = float64(periodMillis) / float64(c.duration)
	}
	scale.amplitudeFactor = focus.gain()
	return scale
}

func resolveTargetContent(target MotionTarget, handyModel string) (MotionTarget, resolvedContent) {
	if target.Dynamic != nil {
		definition := NormalizeDynamicDefinition(*target.Dynamic)
		target.Dynamic = &definition
		target.PatternID = ""
		target.PatternName = DynamicMotionName
		target.ProgramID = ""
		target.MediaID = ""
		target.Pattern = nil
		target.Program = nil
		target.Media = nil
		return target, dynamicContent(definition)
	}
	if target.Media != nil {
		if definition, err := NormalizeMediaTimelineDefinition(*target.Media); err == nil {
			if target.MediaSpeedLimitEnabled {
				definition.Points = limitMediaTimelineRate(definition.Points, target.SpeedPercent, handyModel)
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
		definition, _ = BuiltinPatternDefinition(PatternFullSweeps)
		target.PatternID = PatternFullSweeps
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
// unchanged while bounding only physically over-fast travel. It limits each
// authored displacement rather than chasing the authored absolute target.
// Chasing the target can keep moving quickly in the old direction after the
// script has reversed, which both amplifies that segment and feels faster than
// the uncapped script. Delta limiting preserves every authored direction and
// never adds travel that the source did not request.
func limitMediaTimelineRate(points []CurvePoint, speedPercent int, handyModel string) []CurvePoint {
	if len(points) < 2 {
		return append([]CurvePoint(nil), points...)
	}
	maximumRate := referenceTravelRateForSpeed(speedPercent, handyModel)
	limited := append([]CurvePoint(nil), points...)
	for index := 1; index < len(limited); index++ {
		elapsedMillis := points[index].TimeMillis - points[index-1].TimeMillis
		maximumDelta := maximumRate * float64(elapsedMillis) / 1000
		authoredDelta := points[index].PositionPercent - points[index-1].PositionPercent
		limitedDelta := math.Max(
			-maximumDelta,
			math.Min(maximumDelta, authoredDelta),
		)
		limited[index].PositionPercent = math.Max(
			0,
			math.Min(100, limited[index-1].PositionPercent+limitedDelta),
		)
	}
	return limited
}

func periodForContent(points []CurvePoint, baseDuration int64, speedPercent int, loop bool, handyModel string) int64 {
	speedPercent = clamp(speedPercent, 1, 100)
	minimum := int64(minimumBurstCycleMillis)
	if !loop {
		period := int64(math.Round(float64(baseDuration) * 100 / float64(speedPercent)))
		return max(period, minimum)
	}

	travel := totalCurveTravel(points)
	period := int64(0)
	if travel > 0 {
		requestedRate := referenceTravelRateForSpeed(speedPercent, handyModel)
		period = int64(math.Round(travel * 1000 / requestedRate))
	} else {
		period = int64(math.Round(float64(baseDuration) * 100 / float64(speedPercent)))
	}
	minimum = minimumSafeLoopPeriod(points, baseDuration)
	return max(period, minimum)
}

// referenceTravelRateForSpeed maps the selectable 1..100 control onto the
// selected Handy model's published full-travel, normal speed envelope. The
// result remains semantic percentage-points/second; no physical units or raw
// device payload cross the shared engine boundary. The affine floor matters:
// one is the slowest supported carriage speed, not near-stationary motion.
func referenceTravelRateForSpeed(speedPercent int, handyModel string) float64 {
	speedPercent = clamp(speedPercent, 1, 100)
	progress := float64(speedPercent-1) / 99
	profile := handySpeedProfileFor(handyModel)
	physicalRate := profile.minimumMMPerSecond +
		(profile.maximumMMPerSecond-profile.minimumMMPerSecond)*progress
	return physicalRate / profile.strokeMillimeters * 100
}

func handySpeedProfileFor(handyModel string) handySpeedProfile {
	switch handyModel {
	case config.HandyModel2Standard:
		return handySpeedProfile{strokeMillimeters: 125, minimumMMPerSecond: 32, maximumMMPerSecond: 400}
	case config.HandyModel2Pro:
		return handySpeedProfile{strokeMillimeters: 125, minimumMMPerSecond: 32, maximumMMPerSecond: 450}
	default:
		return handySpeedProfile{strokeMillimeters: 110, minimumMMPerSecond: 32, maximumMMPerSecond: 400}
	}
}

func totalCurveTravel(points []CurvePoint) float64 {
	travel := 0.0
	for index := 1; index < len(points); index++ {
		travel += math.Abs(points[index].PositionPercent - points[index-1].PositionPercent)
	}
	return travel
}

// minimumSafeLoopPeriod returns the shortest full-span playback period that
// keeps the rendered curve inside the runtime acceleration and reversal
// envelope. Catalog authoring remains deliberately gentler. Runtime evaluates
// the actual scaled curve because reversal guides adapt to playback speed; the
// old square-root scaling assumed a fixed authored curve and therefore slowed
// high requested speeds even after the guide had been rebuilt for them.
func minimumSafeLoopPeriod(points []CurvePoint, authoredPeriod int64) int64 {
	return minimumSafeLoopPeriodAtGain(points, authoredPeriod, 1)
}

func minimumSafeLoopPeriodAtGain(points []CurvePoint, authoredPeriod int64, gain float64) int64 {
	return minimumSafeLoopPeriodAtGainWithReversalProfile(
		points, authoredPeriod, gain, curveReversalBoundedRamp,
	)
}

func minimumSafeLoopPeriodAtGainWithReversalProfile(
	points []CurvePoint,
	authoredPeriod int64,
	gain float64,
	reversalProfile curveReversalProfile,
) int64 {
	return minimumSafeLoopPeriodAtGainWithReversalProfileAndMinimum(
		points, authoredPeriod, gain, reversalProfile, minimumBurstCycleMillis,
	)
}

func minimumSafeLoopPeriodAtGainWithReversalProfileAndMinimum(
	points []CurvePoint,
	authoredPeriod int64,
	gain float64,
	reversalProfile curveReversalProfile,
	minimum int64,
) int64 {
	minimum = max(int64(1), minimum)
	if authoredPeriod <= 0 || len(points) < 2 {
		return minimum
	}
	gain = math.Max(0, gain)
	if gain == 0 {
		return minimum
	}
	if gap := reversalGap(points, authoredPeriod, true); gap > 0 {
		minimum = max(minimum, int64(math.Ceil(
			float64(authoredPeriod)*float64(runtimeMinimumReversalGapMillis)/float64(gap),
		)))
	}
	if loopPeriodWithinRuntimeEnvelope(
		points, authoredPeriod, minimum, gain, reversalProfile,
	) {
		return minimum
	}

	upper := max(authoredPeriod, minimum+1)
	for !loopPeriodWithinRuntimeEnvelope(
		points, authoredPeriod, upper, gain, reversalProfile,
	) {
		if upper > math.MaxInt64/2 {
			return upper
		}
		upper *= 2
	}
	lower := minimum
	for lower+1 < upper {
		candidate := lower + (upper-lower)/2
		if loopPeriodWithinRuntimeEnvelope(
			points, authoredPeriod, candidate, gain, reversalProfile,
		) {
			upper = candidate
		} else {
			lower = candidate
		}
	}
	return upper
}

func loopPeriodWithinRuntimeEnvelope(
	points []CurvePoint,
	authoredPeriod, playedPeriod int64,
	gain float64,
	reversalProfile curveReversalProfile,
) bool {
	scale := playbackScale{
		timeFactor: float64(playedPeriod) / float64(authoredPeriod), amplitudeFactor: gain,
		maxAccelerationPercentSecond2: runtimeMaxAccelerationPercentPerSecond2,
	}
	curve, err := newCurveWithReversalProfile(
		points, authoredPeriod, true, false, maximumCurvePoints, scale,
		reversalProfile,
	)
	if err != nil {
		return false
	}
	playedAcceleration := curve.maximumAccelerationPerMillis2() * gain /
		(scale.timeFactor * scale.timeFactor) * 1e6
	if playedAcceleration > runtimeMaxAccelerationPercentPerSecond2*1.001 {
		return false
	}
	if reversalProfile != curveReversalC2Flow {
		return true
	}
	playedJerk := curve.maximumJerkPerMillis3() * gain /
		(scale.timeFactor * scale.timeFactor * scale.timeFactor) * 1e9
	return playedJerk <= runtimeMaxJerkPercentPerSecond3*1.001
}

// focusedLoopPeriod keeps focus and soft anchoring from changing the requested
// mean travel rate. The requested period scales with played amplitude. The
// safety floor treats acceleration and reversal cadence separately: amplitude
// changes acceleration, but the minimum time between reversals does not shrink.
func focusedLoopPeriodWithMinimum(
	period int64,
	points []CurvePoint,
	authoredPeriod int64,
	loop bool,
	focus focusProjection,
	reversalProfile curveReversalProfile,
	minimumPeriod int64,
) int64 {
	gain := focus.gain()
	if !loop || gain <= 0 {
		return period
	}
	adjusted := int64(math.Round(float64(period) * gain))
	minimum := minimumSafeLoopPeriodAtGainWithReversalProfileAndMinimum(
		points, authoredPeriod, gain, reversalProfile, minimumPeriod,
	)
	return max(adjusted, minimum)
}

func motionContentID(target MotionTarget) string {
	if target.Dynamic != nil {
		return dynamicContentID(*target.Dynamic)
	}
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
	gain := f.targetSpan / f.sourceSpan
	if f.anchor != nil {
		gain *= 1 - float64(f.anchor.WeightPercent)/100
	}
	return math.Max(0, gain)
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

func chooseNearestPhase(target MotionTarget, settings config.MotionSettings, current float64, currentDirection int, currentVelocity float64) float64 {
	candidatePlan := NewMotionPlan("candidate", target, settings, 0, 0, time.Unix(0, 0))
	if candidatePlan.compilationError() != nil {
		return 0
	}
	bestPhase := 0.0
	bestDistance := math.MaxFloat64
	// Short catalog motifs were well covered by the original 64-point search,
	// but a Creative span envelope deliberately contains a much longer phrase.
	// Scale the search with authored curve complexity so changing texture does
	// not turn a long loop into a coarse position jump at the handoff.
	searchSamples := clamp(len(candidatePlan.curve.authoredKnots)*4, 64, 4096)
	lastIndex := searchSamples - 1
	if !candidatePlan.Loop {
		lastIndex = searchSamples
	}
	for index := range lastIndex + 1 {
		phase := float64(index) / float64(searchSamples)
		candidateMillis := int64(math.Round(float64(candidatePlan.PeriodMillis) * phase))
		position := candidatePlan.SampleAt(candidateMillis).PositionPercent
		distance := math.Abs(position - current)
		// A replacement plan cannot sample before its handoff: PhaseAt clamps
		// negative elapsed time to zero. Score the same forward-looking guide
		// that DirectionAt and VelocityAt will observe at the new plan's first
		// instant, rather than a centered guide that crosses a reversal the new
		// plan has not actually played.
		const guideMillis = int64(25)
		forwardPosition := candidatePlan.SampleAt(candidateMillis + guideMillis).PositionPercent
		candidateDirection := 0
		switch {
		case forwardPosition > position:
			candidateDirection = 1
		case forwardPosition < position:
			candidateDirection = -1
		}
		candidateVelocity := (forwardPosition - position) * 1000 / float64(guideMillis*2)
		score := handoffScore(distance, currentDirection, candidateDirection, currentVelocity, candidateVelocity)
		if score < bestDistance {
			bestDistance = score
			// Preserve the exact millisecond that was scored. Reconstructing the
			// raw grid fraction can land on the other side of a reversal after
			// period scaling and change the handoff direction.
			bestPhase = float64(candidateMillis) / float64(candidatePlan.PeriodMillis)
		}
	}
	return bestPhase
}

func handoffScore(distance float64, currentDirection, candidateDirection int, currentVelocity, candidateVelocity float64) float64 {
	score := distance
	if currentDirection != 0 && candidateDirection != 0 && candidateDirection != currentDirection {
		// Keep a nearby phase that continues travel ahead of an equally close
		// reversal. This penalty must exceed the capped velocity tie-breaker
		// below; otherwise a near-zero-speed opposite phase can win by a few
		// thousandths of a percent on long Creative curves.
		score += 12
	}
	if candidateDirection == 0 && distance > 2 {
		score += 4
	}
	// Position remains dominant, but two equally close phases should continue
	// with the closest velocity rather than visibly braking or surging.
	score += math.Min(8, math.Abs(candidateVelocity-currentVelocity)*0.025)
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
