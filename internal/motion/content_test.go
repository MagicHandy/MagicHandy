package motion

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestGeneratedCatalogMeetsHardwareBudgets(t *testing.T) {
	definitions := BuiltinPatternDefinitions()
	if len(definitions) < 3 {
		t.Fatalf("catalog size = %d, want baseline patterns", len(definitions))
	}
	for _, definition := range definitions {
		if definition.CycleMillis < RoutineCycleFloorMillis {
			t.Fatalf("pattern %q cycle = %d, below routine floor", definition.ID, definition.CycleMillis)
		}
		if slices.Contains(definition.Tags, TagCurated) {
			continue
		}
		metrics, err := MeasureCurve(definition.Points, definition.CycleMillis, true)
		if err != nil {
			t.Fatalf("measure %q: %v", definition.ID, err)
		}
		if metrics.MaxAccelerationPercentPerSecond2 > catalogMaxAcceleration*1.001 {
			t.Fatalf("pattern %q acceleration = %.2f, budget %.2f", definition.ID, metrics.MaxAccelerationPercentPerSecond2, catalogMaxAcceleration)
		}
		if metrics.MinReversalGapMillis > 0 && metrics.MinReversalGapMillis < catalogMinReversalGap {
			t.Fatalf("pattern %q reversal gap = %d, budget %d", definition.ID, metrics.MinReversalGapMillis, catalogMinReversalGap)
		}
	}
}

func TestMeasureCurveIncludesLoopSeamReversalGap(t *testing.T) {
	points := []CurvePoint{
		{TimeMillis: 0, PositionPercent: 0},
		{TimeMillis: 1000, PositionPercent: 100},
		{TimeMillis: 1200, PositionPercent: 0},
	}
	metrics, err := MeasureCurve(points, 1200, true)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.MinReversalGapMillis != 200 {
		t.Fatalf("loop reversal gap = %dms, want 200ms final-return seam", metrics.MinReversalGapMillis)
	}
}

func TestLoopSpeedNormalizesMeanTravelAcrossPatterns(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	patternIDs := []PatternID{PatternStroke, PatternPulse, PatternHardAndRegular}

	for _, speed := range []int{20, 40} {
		wantRate := referenceTravelRateForSpeed(speed, settings.HandyModel)
		for _, patternID := range patternIDs {
			definition, found := BuiltinPatternDefinition(patternID)
			if !found {
				t.Fatalf("pattern %q is missing", patternID)
			}
			plan := NewMotionPlan("normalized", MotionTarget{
				PatternID: definition.ID, Pattern: &definition, SpeedPercent: speed,
			}, settings, 0, 0, time.Unix(0, 0))
			gotRate := totalCurveTravel(definition.Points) * 1000 / float64(plan.PeriodMillis)
			if math.Abs(gotRate-wantRate) > 0.05 {
				t.Errorf("%s at %d%% = %.2f%% travel/s, want %.2f", definition.Name, speed, gotRate, wantRate)
			}
		}
	}
}

func TestLoopSpeedUsesCalibratedManualControlCurve(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	definition, _ := BuiltinPatternDefinition(PatternStroke)
	period := func(speed int) int64 {
		return NewMotionPlan("speed", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: speed,
		}, settings, 0, 0, time.Unix(0, 0)).PeriodMillis
	}

	for _, pair := range [][2]int{{20, 40}, {40, 73}} {
		slow, fast := period(pair[0]), period(pair[1])
		wantRatio := referenceTravelRateForSpeed(pair[1], settings.HandyModel) /
			referenceTravelRateForSpeed(pair[0], settings.HandyModel)
		if math.Abs(float64(slow)/float64(fast)-wantRatio) > 0.01 {
			t.Fatalf("%d%%/%d%% periods = %d/%d, want calibrated %.3fx tempo change",
				pair[0], pair[1], slow, fast, wantRatio)
		}
	}
}

func TestLoopPlaybackHonorsRuntimeSafetyEnvelope(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	for _, definition := range BuiltinPatternDefinitions() {
		plan := NewMotionPlan("safe", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
		}, settings, 0, 0, time.Unix(0, 0))
		playedAcceleration := maximumPlanAcceleration(plan)
		if playedAcceleration > runtimeMaxAccelerationPercentPerSecond2*1.001 {
			t.Errorf("%s acceleration = %.1f%%/s^2, over %.1f", definition.Name,
				playedAcceleration, runtimeMaxAccelerationPercentPerSecond2)
		}
		metrics, err := MeasureCurve(definition.Points, definition.CycleMillis, true)
		if err != nil {
			t.Fatalf("measure %q: %v", definition.ID, err)
		}
		if metrics.MinReversalGapMillis > 0 {
			playedGap := float64(metrics.MinReversalGapMillis) * float64(plan.PeriodMillis) /
				float64(definition.CycleMillis)
			if playedGap+0.01 < float64(runtimeMinimumReversalGapMillis) {
				t.Errorf("%s reversal gap = %.1fms, below %d", definition.Name,
					playedGap, runtimeMinimumReversalGapMillis)
			}
		}
	}
}

func TestBuiltinMetadataDoesNotEncodeAbsolutePace(t *testing.T) {
	for _, definition := range BuiltinPatternDefinitions() {
		metadata := strings.ToLower(definition.Name + " " + definition.Description + " " + strings.Join(definition.Tags, " "))
		for _, term := range []string{"gentle", "easy", "slow", "steady", "fast", "intense"} {
			if containsMetadataWord(metadata, term) {
				t.Errorf("pattern %q encodes absolute pace %q in model-facing metadata: %q", definition.ID, term, metadata)
			}
		}
	}
}

func containsMetadataWord(metadata, want string) bool {
	for _, field := range strings.FieldsFunc(metadata, func(r rune) bool {
		return r < 'a' || r > 'z'
	}) {
		if field == want {
			return true
		}
	}
	return false
}

func TestExperimentalCatalogContainsSixVariableReplacementCycles(t *testing.T) {
	seenIDs := make(map[PatternID]bool)
	seenShapes := make(map[string]PatternID)
	experimental := 0
	for _, definition := range BuiltinPatternDefinitions() {
		if seenIDs[definition.ID] {
			t.Fatalf("duplicate built-in pattern id %q", definition.ID)
		}
		seenIDs[definition.ID] = true
		if !slices.Contains(definition.Tags, TagExperimental) {
			continue
		}
		assertExperimentalPatternQuality(t, definition)
		if strings.HasPrefix(string(definition.ID), "curated-") {
			continue
		}
		experimental++
		signature := fmt.Sprint(definition.Points)
		if other, exists := seenShapes[signature]; exists {
			t.Fatalf("experimental patterns %q and %q have identical curves", other, definition.ID)
		}
		seenShapes[signature] = definition.ID
	}
	if experimental != 6 {
		t.Fatalf("experimental pattern count = %d, want 6 replacements", experimental)
	}
}

func assertExperimentalPatternQuality(t *testing.T, definition PatternDefinition) {
	t.Helper()
	if !strings.HasPrefix(definition.Description, "Experimental: ") {
		t.Fatalf("experimental pattern %q is not visibly labeled", definition.ID)
	}
	first := definition.Points[0]
	last := definition.Points[len(definition.Points)-1]
	if first.TimeMillis != 0 || last.TimeMillis != definition.CycleMillis || first.PositionPercent != last.PositionPercent {
		t.Fatalf("experimental pattern %q is not a complete closed cycle: first=%+v last=%+v cycle=%d", definition.ID, first, last, definition.CycleMillis)
	}
}

// Stroke-speed envelope, measured from the thirteen patterns that survived the
// user disabling fifteen by hand.
//
// The rule this replaced asked for reach VARIETY -- four amplitude bands, no
// repeated endpoint, no two similar strokes in a row. Measuring the disabled set
// showed variety was never the missing ingredient: they averaged 5.5 distinct
// stroke lengths against 3.0 for the ones that were kept. What separated them was
// speed. Every kept pattern holds at least 44%/s on its slowest stroke; eleven of
// the fifteen disabled ones fall below that, and five stall outright -- Cascade
// spends 2.46s of a 6.6s loop under 30%/s because a shrinking stroke kept a fixed
// half-period.
//
// Speed scaling is a uniform time factor, so these ratios hold at any intensity.
//
// The bounds sit just outside the kept patterns rather than around the new ones:
// Drift runs a 44%/s stroke and both Tease and Deep-Partial Sequence spread 3.2x,
// and those are evidence of what is acceptable, not candidates to reject. That
// costs some detection at the margin -- of the fifteen retired shapes these
// bounds catch twelve, missing Sway, Rolling, and Double Tap, whose slowest
// strokes (46, 54, 57%/s) sit inside the kept range. Those three were always the
// weakest part of the case for removing them.
const (
	envelopeFloorVelocity   = catalogMinStrokeVelocity  // Drift, the slowest kept pattern, runs 44
	envelopeMaxSpeedRatio   = catalogMaxSpeedRatio      // Tease and Deep-Partial Sequence both spread 3.2x
	envelopeMinAmplitude    = catalogMinStrokeAmplitude // reversal gap * floor velocity
	envelopeMinMeanVelocity = catalogMinMeanVelocity    // Drift and Flutter sit at 56-57
	envelopeMaxStallMillis  = catalogMaxStallMillis     // longest contiguous span under 30%/s
)

func TestCatalogPatternsHoldTheMeasuredSpeedEnvelope(t *testing.T) {
	for _, definition := range BuiltinPatternDefinitions() {
		if strings.HasPrefix(string(definition.ID), "curated-") && slices.Contains(definition.Tags, TagExperimental) {
			continue
		}
		// Curated entries are exact user-tested curves; they are evidence, not
		// designs, and deliberately carry holds the envelope would reject.
		if UsesExactImportedCurve(definition) {
			continue
		}
		slowest, fastest, travel := math.Inf(1), 0.0, 0.0
		for index := 1; index < len(definition.Points); index++ {
			previous, point := definition.Points[index-1], definition.Points[index]
			amplitude := math.Abs(point.PositionPercent - previous.PositionPercent)
			millis := point.TimeMillis - previous.TimeMillis
			if amplitude == 0 || millis <= 0 {
				continue
			}
			if amplitude < envelopeMinAmplitude {
				t.Errorf("%s: stroke %d amplitude %.0f below %.0f, too short to stay above the speed floor",
					definition.ID, index, amplitude, envelopeMinAmplitude)
			}
			velocity := amplitude / (float64(millis) / 1000)
			slowest, fastest = math.Min(slowest, velocity), math.Max(fastest, velocity)
			travel += amplitude
		}
		if slowest < envelopeFloorVelocity {
			t.Errorf("%s: slowest stroke %.0f%%/s below the %.0f%%/s floor", definition.ID, slowest, envelopeFloorVelocity)
		}
		if ratio := fastest / slowest; ratio > envelopeMaxSpeedRatio {
			t.Errorf("%s: speed spread %.2fx exceeds %.2fx", definition.ID, ratio, envelopeMaxSpeedRatio)
		}
		if mean := travel / (float64(definition.CycleMillis) / 1000); mean < envelopeMinMeanVelocity {
			t.Errorf("%s: mean speed %.0f%%/s below %.0f%%/s", definition.ID, mean, envelopeMinMeanVelocity)
		}
		if stall := longestStallMillis(t, definition); stall > envelopeMaxStallMillis {
			t.Errorf("%s: stalls for %dms under 30%%/s, above the %dms ceiling",
				definition.ID, stall, envelopeMaxStallMillis)
		}
	}
}

// longestStallMillis reports the longest contiguous span the rendered curve
// spends under 30%/s. Measuring the smoothed curve rather than the authored
// chords is what catches a stall: the device follows the interpolated script.
func longestStallMillis(t *testing.T, definition PatternDefinition) int64 {
	t.Helper()
	curve, err := NewCurve(definition.Points, definition.CycleMillis, true)
	if err != nil {
		t.Fatalf("%s: %v", definition.ID, err)
	}
	const step = 25
	run, longest := int64(0), int64(0)
	for at := int64(0); at < definition.CycleMillis; at += step {
		if math.Abs(curve.Velocity(at)) < 30 {
			run += step
			longest = max(longest, run)
			continue
		}
		run = 0
	}
	return longest
}

func TestOnlyReplacementAndGeneratedReviewPatternsAreExperimental(t *testing.T) {
	want := map[PatternID]bool{
		PatternRisingReach:    true,
		PatternOffbeat:        true,
		PatternLongReturn:     true,
		PatternSwell:          true,
		PatternSurgeAndSettle: true,
		PatternCrosscut:       true,
	}
	for _, definition := range BuiltinPatternDefinitions() {
		hasTag := slices.Contains(definition.Tags, TagExperimental)
		if strings.HasPrefix(string(definition.ID), "curated-") {
			if hasTag != strings.HasPrefix(definition.Description, "Experimental: ") {
				t.Fatalf("generated pattern %q tag/description label mismatch", definition.ID)
			}
			continue
		}
		wantExperimental := want[definition.ID]
		if hasTag != wantExperimental {
			t.Fatalf("pattern %q experimental = %t, want %t", definition.ID, hasTag, wantExperimental)
		}
		if hasTag != strings.HasPrefix(definition.Description, "Experimental: ") {
			t.Fatalf("pattern %q tag/description label mismatch", definition.ID)
		}
	}
}

func TestRetiredPatternsAreAbsentFromDefaultCatalog(t *testing.T) {
	retired := RetiredBuiltinPatternIDs()
	if !slices.Contains(retired, PatternCradle) {
		t.Fatal("Cradle is not recorded as retired after failed physical playback")
	}
	for _, definition := range BuiltinPatternDefinitions() {
		if slices.Contains(retired, definition.ID) {
			t.Fatalf("retired pattern %q remains in the default catalog", definition.ID)
		}
	}
}

func TestSampledPatternsUseMotionSemanticNames(t *testing.T) {
	want := map[PatternID]string{
		PatternFourLevelCircuit:    "Four-Level Circuit",
		PatternHighLowBlocks:       "High-Low Blocks",
		PatternDeepShallowSequence: "Deep-Shallow Sequence",
		PatternSlowFastFull:        "Tempo Ramp",
		PatternDeepPartialSequence: "Deep-Partial Sequence",
		PatternRisingReach:         "Rising Reach",
		PatternEasingDown:          "Descending Window",
		PatternBuildingUp:          "Ascending Window",
		PatternBroadAndTight:       "Broad and Tight",
		PatternUpperAccents:        "Upper Accents",
		PatternLowerAccents:        "Lower Accents",
		PatternSteadyDrift:         "Window Drift",
		PatternNarrowing:           "Narrowing Window",
		PatternOpeningUp:           "Widening Window",
		PatternRocking:             "Rocking",
		PatternThreeAndOne:         "Three and One",
		PatternOffbeat:             "Offbeat",
		PatternLongReturn:          "Long Return",
		PatternSwell:               "Rising Window Arc",
		PatternSurgeAndSettle:      "Full Sweep and Mid Blocks",
		PatternCrosscut:            "Crosscut",
	}
	for _, definition := range BuiltinPatternDefinitions() {
		name, ok := want[definition.ID]
		if !ok {
			continue
		}
		if definition.Name != name || strings.Contains(strings.ToLower(definition.Description), "funscript") {
			t.Fatalf("pattern %q metadata = %q / %q, want motion-semantic metadata", definition.ID, definition.Name, definition.Description)
		}
		delete(want, definition.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing sampled patterns: %+v", want)
	}
}

func TestPromotedUserPatternsKeepAcceptedIdentityAndTimingPolicy(t *testing.T) {
	want := map[PatternID]struct {
		name       string
		cycle      int64
		pointCount int
		exact      bool
	}{
		PatternHardAndRegular: {name: "Hard and Regular", cycle: 7200, pointCount: 49},
		PatternPlayfulJerk:    {name: "playful jerk", cycle: 11704, pointCount: 33, exact: true},
	}
	for _, definition := range PromotedBuiltinPatternDefinitions() {
		expected, ok := want[definition.ID]
		if !ok {
			t.Fatalf("unexpected promoted pattern %q", definition.ID)
		}
		if UsesExactImportedCurve(definition) != expected.exact {
			t.Fatalf("promoted pattern %q exact timing = %t, want %t", definition.ID, UsesExactImportedCurve(definition), expected.exact)
		}
		if definition.Name != expected.name || definition.CycleMillis != expected.cycle || len(definition.Points) != expected.pointCount {
			t.Fatalf("promoted pattern %q = name %q cycle %d points %d", definition.ID, definition.Name, definition.CycleMillis, len(definition.Points))
		}
		if slices.Contains(definition.Tags, TagExperimental) || !slices.Contains(definition.Tags, TagCurated) {
			t.Fatalf("promoted pattern %q tags = %v", definition.ID, definition.Tags)
		}
		delete(want, definition.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing promoted patterns: %+v", want)
	}
	if UsesExactImportedCurve(PatternDefinition{ID: "unreviewed", Tags: []string{TagCurated}}) {
		t.Fatal("unreviewed curated tag bypassed generated-catalog safety budgets")
	}
}

func TestBuiltinPatternCatalogReturnsDefensiveCopies(t *testing.T) {
	definitions := BuiltinPatternDefinitions()
	definitions[0].Points[0].PositionPercent = 99
	definitions[0].Tags[0] = "changed"

	again, ok := BuiltinPatternDefinition(definitions[0].ID)
	if !ok {
		t.Fatal("built-in pattern disappeared")
	}
	if again.Points[0].PositionPercent == 99 || again.Tags[0] == "changed" {
		t.Fatalf("built-in catalog was mutated through returned copy: %+v", again)
	}
}

func TestMonotoneCurveDoesNotOvershootAndStopsAtReversal(t *testing.T) {
	points := []CurvePoint{
		{TimeMillis: 0, PositionPercent: 10},
		{TimeMillis: 1000, PositionPercent: 90},
		{TimeMillis: 2000, PositionPercent: 20},
	}
	curve, err := NewCurve(points, 2000, false)
	if err != nil {
		t.Fatal(err)
	}
	for at := int64(0); at <= 2000; at += 10 {
		value := curve.Sample(at)
		if value < 10 || value > 90 {
			t.Fatalf("sample at %d = %.4f, overshot source range", at, value)
		}
	}
	if velocity := math.Abs(curve.Velocity(1000)); velocity > 0.0001 {
		t.Fatalf("reversal velocity = %.6f, want zero", velocity)
	}
}

func TestMonotoneCurveIsContinuousInWallTime(t *testing.T) {
	curve, err := NewCurve([]CurvePoint{
		{TimeMillis: 0, PositionPercent: 10},
		{TimeMillis: 1000, PositionPercent: 30},
		{TimeMillis: 3000, PositionPercent: 80},
	}, 3000, false)
	if err != nil {
		t.Fatal(err)
	}
	left := curve.Velocity(999)
	right := curve.Velocity(1001)
	if difference := math.Abs(left - right); difference > 0.2 {
		t.Fatalf("velocity jumped %.3f percent/s across wall-time knot: %.3f -> %.3f", difference, left, right)
	}
}

func TestPlanUsesResolvedPatternAndFiniteProgram(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	pattern := PatternDefinition{
		ID: "user-square", Name: "User", Kind: PatternKindRoutine,
		CycleMillis: RoutineCycleFloorMillis,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 30},
			{TimeMillis: RoutineCycleFloorMillis / 2, PositionPercent: 70},
			{TimeMillis: RoutineCycleFloorMillis, PositionPercent: 30},
		},
	}
	plan := NewMotionPlan("custom", MotionTarget{
		PatternID: pattern.ID, Pattern: &pattern, SpeedPercent: 100,
	}, settings, 0, 0, time.Unix(0, 0))
	if got := plan.SampleAt(0).PositionPercent; got != 30 {
		t.Fatalf("resolved custom sample = %g, want 30", got)
	}

	program := ProgramDefinition{
		ID: "program-one", Name: "Program", DurationMillis: 1000,
		Points: []CurvePoint{{TimeMillis: 0}, {TimeMillis: 1000, PositionPercent: 100}},
	}
	finite := NewMotionPlan("finite", MotionTarget{
		ProgramID: program.ID, Program: &program, SpeedPercent: 100,
	}, settings, 0, 0, time.Unix(0, 0))
	if finite.Loop || finite.CompleteAt(999) || !finite.CompleteAt(1000) {
		t.Fatalf("finite completion state is wrong: %+v", finite)
	}
	if got := finite.SampleAt(5000).PositionPercent; got != 100 {
		t.Fatalf("finite endpoint = %g, want held final position", got)
	}
}

func TestEmptyCurveSamplingFailsStationaryInsteadOfPanicking(t *testing.T) {
	curve := Curve{}
	if position := curve.sampleFloat(0); position != 50 {
		t.Fatalf("empty curve position = %g, want safe midpoint", position)
	}
	if velocity := curve.velocityFloat(0); velocity != 0 {
		t.Fatalf("empty curve velocity = %g, want stationary", velocity)
	}
}

func TestInvalidResolvedProgramFallsBackWithoutRetainingProgram(t *testing.T) {
	invalid := ProgramDefinition{ID: "invalid", Name: "Invalid"}
	plan := NewMotionPlan("fallback", MotionTarget{
		ProgramID: invalid.ID,
		Program:   &invalid,
	}, config.DefaultSettings().Motion, 0, 0, time.Unix(0, 0))
	if plan.ProgramID != "" || plan.Target.Program != nil || plan.Target.ProgramID != "" {
		t.Fatalf("fallback plan retained invalid program: %+v", plan)
	}
	if plan.PatternID != PatternStroke || plan.Target.Pattern == nil {
		t.Fatalf("fallback plan = %+v, want resolved stroke pattern", plan)
	}
}

func TestChooseNearestPhaseIncludesFiniteEndpoint(t *testing.T) {
	program := ProgramDefinition{
		ID: "endpoint", Name: "Endpoint", DurationMillis: 1000,
		Points: []CurvePoint{{TimeMillis: 0}, {TimeMillis: 1000, PositionPercent: 100}},
	}
	phase := chooseNearestPhase(MotionTarget{Program: &program, ProgramID: program.ID}, config.DefaultSettings().Motion, 100, 1, 0)
	if phase != 1 {
		t.Fatalf("finite endpoint phase = %g, want 1", phase)
	}
}

func TestMediaTimelineKeepsAuthoredClockAndPreservesSubLimitTravel(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 40
	timeline := MediaTimelineDefinition{
		ID: "video", Name: "Video", DurationMillis: 1000,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 500, PositionPercent: 40},
			{TimeMillis: 1000, PositionPercent: 0},
		},
	}
	plan := NewMotionPlan("media", MotionTarget{
		Label: "Video", Source: TargetSourceMedia, MediaID: timeline.ID, Media: &timeline,
	}, settings, 0, 0, time.Unix(0, 0))

	if plan.PeriodMillis != 1000 || plan.Loop {
		t.Fatalf("media plan timing = period %d loop %v", plan.PeriodMillis, plan.Loop)
	}
	if plan.Target.SpeedPercent != 40 {
		t.Fatalf("media speed limit = %d, want configured maximum 40", plan.Target.SpeedPercent)
	}
	if plan.Target.MediaSpeedLimitEnabled {
		t.Fatal("media speed limit unexpectedly enabled by default")
	}
	if got := plan.SampleAt(125).PositionPercent; math.Abs(got-10) > 0.001 {
		t.Fatalf("sub-limit authored sample = %.3f, want 10", got)
	}
}

func TestMediaTimelineCapsOverFastTravelWithoutChangingClock(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 40
	settings.ApplyVideoSpeedLimit = true
	timeline := MediaTimelineDefinition{
		ID: "fast-video", Name: "Fast video", DurationMillis: 200,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 100, PositionPercent: 100},
			{TimeMillis: 200, PositionPercent: 0},
		},
	}
	plan := NewMotionPlan("fast-media", MotionTarget{
		Label: "Fast video", Source: TargetSourceMedia, MediaID: timeline.ID, Media: &timeline,
	}, settings, 0, 0, time.Unix(0, 0))

	if plan.PeriodMillis != timeline.DurationMillis || plan.Loop {
		t.Fatalf("media plan timing = period %d loop %v", plan.PeriodMillis, plan.Loop)
	}
	maximumDelta := referenceTravelRateForSpeed(40, settings.HandyModel) / 10
	want := []CurvePoint{
		{TimeMillis: 0, PositionPercent: 0},
		{TimeMillis: 100, PositionPercent: maximumDelta},
		{TimeMillis: 200, PositionPercent: 0},
	}
	for index, point := range plan.Target.Media.Points {
		if point.TimeMillis != want[index].TimeMillis || math.Abs(point.PositionPercent-want[index].PositionPercent) > 1e-9 {
			t.Fatalf("limited point %d = %+v, want %+v", index, point, want[index])
		}
	}
}

func TestMediaTimelineLimitPreservesAuthoredDirectionAfterClipping(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 40
	settings.ApplyVideoSpeedLimit = true
	timeline := MediaTimelineDefinition{
		ID: "reversal-after-clip", Name: "Reversal after clip", DurationMillis: 300,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 100, PositionPercent: 100},
			{TimeMillis: 200, PositionPercent: 90},
			{TimeMillis: 300, PositionPercent: 95},
		},
	}
	plan := NewMotionPlan("limited-reversal", MotionTarget{
		Label: "Reversal after clip", Source: TargetSourceMedia, MediaID: timeline.ID, Media: &timeline,
	}, settings, 0, 0, time.Unix(0, 0))

	maximumDelta := referenceTravelRateForSpeed(40, settings.HandyModel) / 10
	want := []CurvePoint{
		{TimeMillis: 0, PositionPercent: 0},
		{TimeMillis: 100, PositionPercent: maximumDelta},
		{TimeMillis: 200, PositionPercent: maximumDelta - 10},
		{TimeMillis: 300, PositionPercent: maximumDelta - 5},
	}
	for index, point := range plan.Target.Media.Points {
		if point.TimeMillis != want[index].TimeMillis || math.Abs(point.PositionPercent-want[index].PositionPercent) > 1e-9 {
			t.Fatalf("limited point %d = %+v, want %+v", index, point, want[index])
		}
	}
}

func TestMediaTimelineLimitNeverAmplifiesAnAuthoredSegment(t *testing.T) {
	points := []CurvePoint{
		{TimeMillis: 0, PositionPercent: 50},
		{TimeMillis: 80, PositionPercent: 100},
		{TimeMillis: 420, PositionPercent: 92},
		{TimeMillis: 500, PositionPercent: 5},
		{TimeMillis: 1200, PositionPercent: 40},
		{TimeMillis: 1300, PositionPercent: 40},
	}
	limited := limitMediaTimelineRate(points, 25, config.HandyModelOriginal)
	maximumRate := referenceTravelRateForSpeed(25, config.HandyModelOriginal)

	for index := 1; index < len(points); index++ {
		authoredDelta := points[index].PositionPercent - points[index-1].PositionPercent
		limitedDelta := limited[index].PositionPercent - limited[index-1].PositionPercent
		maximumDelta := maximumRate * float64(points[index].TimeMillis-points[index-1].TimeMillis) / 1000
		if math.Abs(limitedDelta) > maximumDelta+1e-9 {
			t.Fatalf("segment %d limited delta %.3f exceeds rate cap %.3f", index, limitedDelta, maximumDelta)
		}
		if math.Abs(limitedDelta) > math.Abs(authoredDelta)+1e-9 {
			t.Fatalf("segment %d limited delta %.3f amplifies authored %.3f", index, limitedDelta, authoredDelta)
		}
		if authoredDelta*limitedDelta < 0 {
			t.Fatalf("segment %d reversed direction: authored %.3f, limited %.3f", index, authoredDelta, limitedDelta)
		}
		if limited[index].PositionPercent < 0 || limited[index].PositionPercent > 100 {
			t.Fatalf("segment %d position %.3f is outside semantic travel", index, limited[index].PositionPercent)
		}
	}
}

func TestMediaTimelineSupportsFeatureLengthPointCounts(t *testing.T) {
	points := make([]CurvePoint, MaximumMediaTimelinePoints)
	for index := range points {
		points[index] = CurvePoint{TimeMillis: int64(index), PositionPercent: float64(index % 101)}
	}
	timeline, err := NormalizeMediaTimelineDefinition(MediaTimelineDefinition{
		ID: "feature", Name: "Feature", DurationMillis: int64(len(points) - 1), Points: points,
	})
	if err != nil {
		t.Fatalf("NormalizeMediaTimelineDefinition: %v", err)
	}
	if len(timeline.Points) != MaximumMediaTimelinePoints {
		t.Fatalf("timeline points = %d", len(timeline.Points))
	}
	if _, err := NewCurve(points[:maximumCurvePoints+1], maximumCurvePoints, false); err == nil {
		t.Fatal("normal motion curve unexpectedly accepted media-sized content")
	}
	overLimit := append(points, CurvePoint{TimeMillis: int64(len(points)), PositionPercent: 50})
	if _, err := NormalizeMediaTimelineDefinition(MediaTimelineDefinition{
		ID: "too-large", Name: "Too large", DurationMillis: int64(len(overLimit) - 1), Points: overLimit,
	}); err == nil {
		t.Fatal("media timeline accepted content over its documented point bound")
	}
}

func TestMotionPlanPreservesFractionalSamplePosition(t *testing.T) {
	curve, err := NewCurve([]CurvePoint{
		{TimeMillis: 0, PositionPercent: 0},
		{TimeMillis: 1000, PositionPercent: 100},
	}, 1000, false)
	if err != nil {
		t.Fatalf("NewCurve: %v", err)
	}
	plan := MotionPlan{ID: "fractional", PeriodMillis: 1000, curve: curve}
	got := plan.SampleAt(333).PositionPercent
	want := curve.Sample(333)
	if math.Abs(got-want) > 0.000001 || got == math.Round(got) {
		t.Fatalf("sample position = %.9f, want fractional curve value %.9f without sampler rounding", got, want)
	}
}

func TestNormalizePatternDefinitionRemovesLegacyReversalChatter(t *testing.T) {
	definition, err := NormalizePatternDefinition(PatternDefinition{
		ID: "legacy-chatter", Name: "Legacy chatter", Kind: PatternKindRoutine, CycleMillis: 6600,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 1000, PositionPercent: 20},
			{TimeMillis: 1100, PositionPercent: 19},
			{TimeMillis: 2000, PositionPercent: 30},
			{TimeMillis: 6600, PositionPercent: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	anchors := curveReversalAnchors(definition.Points)
	if len(anchors) != 3 || definition.Points[anchors[1]].PositionPercent != 30 {
		t.Fatalf("normalized points = %+v anchors = %v, want only the meaningful 30%% reversal", definition.Points, anchors)
	}
}

func TestNormalizePatternDefinitionPreservesSlowSubtleReversal(t *testing.T) {
	definition, err := NormalizePatternDefinition(PatternDefinition{
		ID: "slow-subtle", Name: "Slow subtle", Kind: PatternKindRoutine, CycleMillis: 6600,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 1000, PositionPercent: 20},
			{TimeMillis: 2000, PositionPercent: 19},
			{TimeMillis: 3000, PositionPercent: 30},
			{TimeMillis: 6600, PositionPercent: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(definition.Points, func(point CurvePoint) bool {
		return point.TimeMillis == 2000 && point.PositionPercent == 19
	}) {
		t.Fatalf("normalized points = %+v, want slow 1%% reversal preserved", definition.Points)
	}
}

func TestLoopCurveKeepsVelocityAcrossMonotonicSeam(t *testing.T) {
	curve, err := NewCurve([]CurvePoint{
		{TimeMillis: 0, PositionPercent: 50},
		{TimeMillis: 1000, PositionPercent: 70},
		{TimeMillis: 2000, PositionPercent: 30},
		{TimeMillis: 3000, PositionPercent: 50},
	}, 3000, true)
	if err != nil {
		t.Fatal(err)
	}
	// The ramp is an acceleration budget, not a constant: a 20% leg over
	// 1000ms only needs b*(1000-b) = 20/0.002, so b rounds up to 11ms and the
	// cruise body keeps almost the whole leg.
	const wantBlendMillis = 11
	wantVelocity := 20.0 / (1 - wantBlendMillis/2000.0)
	if velocity := curve.Velocity(0); math.Abs(velocity-wantVelocity) > 0.001 {
		t.Fatalf("seam velocity = %.6f%%/s, want continuous %.6f%%/s", velocity, wantVelocity)
	}
	if difference := math.Abs(curve.Velocity(1) - curve.Velocity(2999)); difference > 0.2 {
		t.Fatalf("velocity jumps %.3f%%/s across monotonic loop seam", difference)
	}
}

func TestLoopCurveStopsAtSeamReversal(t *testing.T) {
	curve, err := NewCurve([]CurvePoint{
		{TimeMillis: 0, PositionPercent: 20},
		{TimeMillis: 1000, PositionPercent: 80},
		{TimeMillis: 2000, PositionPercent: 20},
	}, 2000, true)
	if err != nil {
		t.Fatal(err)
	}
	if velocity := math.Abs(curve.Velocity(0)); velocity > 0.001 {
		t.Fatalf("reversing seam velocity = %.6f%%/s, want zero", velocity)
	}
}

func TestLoopCurveConfinesReversalEasingToApex(t *testing.T) {
	curve, err := NewCurve([]CurvePoint{
		{TimeMillis: 0, PositionPercent: 0},
		{TimeMillis: 1000, PositionPercent: 100},
		{TimeMillis: 2000, PositionPercent: 0},
	}, 2000, true)
	if err != nil {
		t.Fatal(err)
	}
	// A full 0-100 leg over 1000ms needs b*(1000-b) = 100/0.002, so both ramps
	// are 53ms and the 947ms cruise runs at 100/947 %/ms. A quarter-cycle
	// therefore sits at 53*(100/947)/2 + 197*(100/947) = 23.601%.
	for _, sample := range []struct {
		at   int64
		want float64
	}{
		{at: 250, want: 23.601},
		{at: 750, want: 76.399},
		{at: 1250, want: 76.399},
		{at: 1750, want: 23.601},
	} {
		if got := curve.Sample(sample.at); math.Abs(got-sample.want) > 0.01 {
			t.Fatalf("sample at %dms = %.3f, want bounded-ease %.3f away from reversal", sample.at, got, sample.want)
		}
	}
	for _, at := range []int64{0, 1000} {
		if velocity := math.Abs(curve.Velocity(at)); velocity > 0.001 {
			t.Fatalf("reversal velocity at %dms = %.6f%%/s, want zero", at, velocity)
		}
	}
}
