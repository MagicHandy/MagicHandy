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

func TestExperimentalCatalogContainsSixVariableClosedCycles(t *testing.T) {
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
		experimental++
		assertExperimentalPatternQuality(t, definition)
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
	minimum, maximum := first.PositionPercent, first.PositionPercent
	amplitudeBands := make(map[int]bool)
	positionUses := make(map[int]int)
	repeatedAmplitudeRun := 1
	longestAmplitudeRun := 1
	previousAmplitude := -1.0
	for index, point := range definition.Points[:len(definition.Points)-1] {
		minimum = math.Min(minimum, point.PositionPercent)
		maximum = math.Max(maximum, point.PositionPercent)
		if index > 0 && point.PositionPercent == definition.Points[index-1].PositionPercent {
			t.Fatalf("experimental pattern %q has a stationary adjacent knot at %d", definition.ID, index)
		}
		next := definition.Points[index+1]
		amplitude := math.Abs(next.PositionPercent - point.PositionPercent)
		if amplitude < 30 {
			t.Fatalf("experimental pattern %q travel %d amplitude = %.1f, want at least 30", definition.ID, index, amplitude)
		}
		amplitudeBands[int(math.Round(amplitude/10))] = true
		positionUses[int(math.Round(point.PositionPercent/5))]++
		if math.Abs(amplitude-previousAmplitude) < 5 {
			repeatedAmplitudeRun++
		} else {
			repeatedAmplitudeRun = 1
		}
		longestAmplitudeRun = max(longestAmplitudeRun, repeatedAmplitudeRun)
		previousAmplitude = amplitude
	}
	if maximum-minimum < 65 {
		t.Fatalf("experimental pattern %q span = %.1f, want a meaningful motion range", definition.ID, maximum-minimum)
	}
	if len(amplitudeBands) < 4 || longestAmplitudeRun > 2 {
		t.Fatalf("experimental pattern %q has weak reach variation: bands=%d longest run=%d", definition.ID, len(amplitudeBands), longestAmplitudeRun)
	}
	for band, uses := range positionUses {
		if uses > 2 {
			t.Fatalf("experimental pattern %q repeats endpoint band %d %d times", definition.ID, band, uses)
		}
	}
}

func TestOnlyReplacementPatternsAreExperimental(t *testing.T) {
	want := map[PatternID]bool{
		PatternDeepMediumShortPairs: true,
		PatternFallingCrest:         true,
		PatternThreeDeepOneShort:    true,
		PatternDescendingLadder:     true,
		PatternWanderingSwell:       true,
		PatternRisingReach:          true,
	}
	for _, definition := range BuiltinPatternDefinitions() {
		hasTag := slices.Contains(definition.Tags, TagExperimental)
		if hasTag != want[definition.ID] {
			t.Fatalf("pattern %q experimental = %t, want %t", definition.ID, hasTag, want[definition.ID])
		}
		if hasTag != strings.HasPrefix(definition.Description, "Experimental: ") {
			t.Fatalf("pattern %q tag/description label mismatch", definition.ID)
		}
	}
}

func TestSampledPatternsUseMotionSemanticNames(t *testing.T) {
	want := map[PatternID]string{
		PatternFourLevelCircuit:     "Four-Level Circuit",
		PatternHighLowBlocks:        "High-Low Blocks",
		PatternDeepShallowSequence:  "Deep-Shallow Sequence",
		PatternShortMediumSteps:     "Short-Medium Steps",
		PatternDeepMediumShortPairs: "Deep, Medium, Short",
		PatternFallingCrest:         "Falling Crest",
		PatternThreeDeepOneShort:    "Three Deep, One Short",
		PatternDescendingLadder:     "Descending Ladder",
		PatternSlowFastFull:         "Slow-to-Fast Full",
		PatternWanderingSwell:       "Wandering Swell",
		PatternDeepPartialSequence:  "Deep-Partial Sequence",
		PatternRisingReach:          "Rising Reach",
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

func TestPromotedUserPatternsKeepAcceptedNamesAndTiming(t *testing.T) {
	want := map[PatternID]struct {
		name       string
		cycle      int64
		pointCount int
	}{
		PatternHardAndRegular: {name: "Hard and Regular", cycle: 7333, pointCount: 49},
		PatternPlayfulJerk:    {name: "playful jerk", cycle: 11704, pointCount: 33},
	}
	for _, definition := range PromotedBuiltinPatternDefinitions() {
		expected, ok := want[definition.ID]
		if !ok {
			t.Fatalf("unexpected promoted pattern %q", definition.ID)
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
	phase := chooseNearestPhase(MotionTarget{Program: &program, ProgramID: program.ID}, config.DefaultSettings().Motion, 100, 1)
	if phase != 1 {
		t.Fatalf("finite endpoint phase = %g, want 1", phase)
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
