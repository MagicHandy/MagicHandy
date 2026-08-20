package motion

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestEnginePreservesRetimedAuthoredKnotsBetweenSamplerTicks(t *testing.T) {
	fake := transport.NewFake()
	engine, err := NewEngine(EngineOptions{
		Transport:        fake,
		DispatchInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	if _, err := engine.Start(context.Background(), MotionTarget{
		PatternID: PatternHardAndRegular, SpeedPercent: 100,
	}, settings); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	add := lastPointsAdd(fake.Commands())
	if add == nil {
		t.Fatal("start did not append points")
	}
	definition, found := BuiltinPatternDefinition(PatternHardAndRegular)
	if !found {
		t.Fatal("Hard and Regular pattern is missing")
	}
	outputTimes := pointTimes(add.Points)
	for _, point := range definition.Points[1:6] {
		authoredTime := point.TimeMillis
		playedTime := int64(math.Round(
			float64(authoredTime) * float64(engine.plan.PeriodMillis) / float64(definition.CycleMillis),
		))
		for engine.nextSampleMillis <= playedTime {
			engine.mu.Lock()
			samples, sampleErr := engine.nextMotionSamplesLocked()
			engine.mu.Unlock()
			if sampleErr != nil {
				t.Fatalf("sample follow-up chunk: %v", sampleErr)
			}
			for _, sample := range samples {
				outputTimes = append(outputTimes, sample.TimeMillis)
			}
		}
		if !containsMillis(outputTimes, playedTime) {
			t.Fatalf("output times = %v, missing retimed knot %d (authored at %d)", outputTimes, playedTime, authoredTime)
		}
	}
}

func TestEngineFrameHonorsImmediateModeTimingFloor(t *testing.T) {
	commandTransport := &timingCapabilityTransport{
		Fake:            transport.NewFake(),
		minimumInterval: 300 * time.Millisecond,
	}
	engine, err := NewEngine(EngineOptions{
		Transport: commandTransport, DispatchInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	if _, err := engine.Start(context.Background(), MotionTarget{
		PatternID: PatternHardAndRegular, SpeedPercent: 100,
	}, settings); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	add := lastPointsAdd(commandTransport.Commands())
	if add == nil || len(add.Points) < 2 {
		t.Fatalf("points = %+v, want a playable frame", add)
	}
	for index := 1; index < len(add.Points); index++ {
		gap := add.Points[index].TimeMillis - add.Points[index-1].TimeMillis
		if gap < commandTransport.minimumInterval.Milliseconds() {
			t.Fatalf("point gap %d = %dms, below transport floor %dms", index, gap, commandTransport.minimumInterval.Milliseconds())
		}
	}
}

func TestEngineReadsOwnerPositionResolution(t *testing.T) {
	commandTransport := &samplingCapabilityTransport{
		Fake: transport.NewFake(), resolution: 1, afterStrokeWindow: true, maximumPoints: 100,
	}
	engine, err := NewEngine(EngineOptions{Transport: commandTransport})
	if err != nil {
		t.Fatal(err)
	}
	if engine.positionResolutionPercent != 1 {
		t.Fatalf("position resolution = %g, want owner-advertised 1%%", engine.positionResolutionPercent)
	}
	if !engine.resolutionAfterStrokeWindow || engine.maximumChunkPoints != 100 {
		t.Fatalf("sampling capabilities were not retained: %+v", engine)
	}
	settings := config.DefaultSettings().Motion
	settings.StrokeMinPercent = 20
	settings.StrokeMaxPercent = 80
	engine.settings = settings
	if got := engine.effectivePositionResolutionPercentLocked(); math.Abs(got-5.0/3.0) > 1e-12 {
		t.Fatalf("effective 60%%-window resolution = %g, want 5/3%%", got)
	}
}

func TestLinearMediaSamplerDoesNotInsertChunkBoundaryPoints(t *testing.T) {
	timeline := MediaTimelineDefinition{
		ID: "reported-media", Name: "Reported media", DurationMillis: 7546,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 46},
			{TimeMillis: 977, PositionPercent: 81},
			{TimeMillis: 1931, PositionPercent: 41},
			{TimeMillis: 2855, PositionPercent: 66},
			{TimeMillis: 3831, PositionPercent: 37},
			{TimeMillis: 4823, PositionPercent: 67},
			{TimeMillis: 5938, PositionPercent: 35},
			{TimeMillis: 6629, PositionPercent: 70},
			{TimeMillis: 7546, PositionPercent: 41},
		},
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 30
	plan := NewMotionPlan("reported-media", MotionTarget{
		Label: "Reported media", Source: TargetSourceMedia, SpeedPercent: 30,
		Media: &timeline,
	}, settings, 0, 0, time.Unix(0, 0))
	engine := &Engine{
		plan: plan, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true, maximumChunkPoints: 100,
	}

	first, err := engine.nextLinearMediaSamplesLocked(5000)
	if err != nil {
		t.Fatal(err)
	}
	assertMotionSampleTimes(t, first, []int64{0, 977, 1931, 2855, 3831, 4823, 5938})
	wantPositions := []float64{46, 81, 41, 66, 37, 67, 35}
	for index, sample := range first {
		if math.Abs(sample.PositionPercent-wantPositions[index]) > 0.001 {
			t.Fatalf("sample %d position = %.3f, want authored %.3f", index, sample.PositionPercent, wantPositions[index])
		}
	}
	for _, boundary := range []int64{1000, 2000, 3000, 4000, 5000} {
		for _, sample := range first {
			if sample.TimeMillis == boundary {
				t.Fatalf("samples include unrelated chunk boundary %d: %+v", boundary, first)
			}
		}
	}

	second, err := engine.nextLinearMediaSamplesLocked(10000)
	if err != nil {
		t.Fatal(err)
	}
	assertMotionSampleTimes(t, second, []int64{6629, 7546, 10000})
	if got, want := second[len(second)-1].PositionPercent, plan.SampleAt(10000).PositionPercent; got != want {
		t.Fatalf("terminal hold position = %g, want %g", got, want)
	}
}

func assertMotionSampleTimes(t *testing.T, samples []MotionSample, want []int64) {
	t.Helper()
	if len(samples) != len(want) {
		t.Fatalf("sample times = %v, want %v", motionSampleTimes(samples), want)
	}
	for index, sample := range samples {
		if sample.TimeMillis != want[index] {
			t.Fatalf("sample times = %v, want %v", motionSampleTimes(samples), want)
		}
	}
}

func motionSampleTimes(samples []MotionSample) []int64 {
	times := make([]int64, len(samples))
	for index, sample := range samples {
		times[index] = sample.TimeMillis
	}
	return times
}

func TestAdaptiveSamplesBoundLinearApproximation(t *testing.T) {
	definition := PatternDefinition{
		ID: "subtle", Name: "Subtle", Kind: PatternKindRoutine, CycleMillis: 6600,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 3300, PositionPercent: 100},
			{TimeMillis: 6600, PositionPercent: 0},
		},
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	plan := NewMotionPlan("subtle", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
		AreaFocus: &AreaFocus{MinPercent: 45, MaxPercent: 55},
	}, settings, 0, 0, time.Unix(0, 0))
	engine := &Engine{
		plan: plan, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true,
	}
	samples, err := engine.nextMotionSamplesLocked()
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) >= defaultChunkSize {
		t.Fatalf("adaptive output kept %d points, want fewer than fixed %d-point frame", len(samples), defaultChunkSize)
	}
	for at := samples[0].TimeMillis; at <= samples[len(samples)-1].TimeMillis; at += 5 {
		got := interpolateMotionSamples(samples, at)
		want := plan.SampleAt(at).PositionPercent
		if delta := math.Abs(got - want); delta > wireApproximationTolerance+0.01 {
			t.Fatalf("linearized sample at %d differs by %.3f%%, want <= %.3f%%", at, delta, wireApproximationTolerance)
		}
	}
}

func TestAdaptiveFrameRejectsPathologicallyDenseEssentialContent(t *testing.T) {
	points := make([]CurvePoint, 201)
	for index := range points {
		position := 0.0
		if index%2 == 1 {
			position = 100
		}
		points[index] = CurvePoint{TimeMillis: int64(index * 5), PositionPercent: position}
	}
	definition := ProgramDefinition{
		ID: "dense", Name: "Dense", DurationMillis: 1000, Points: points,
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	plan := NewMotionPlan("dense", MotionTarget{
		ProgramID: definition.ID, Program: &definition, SpeedPercent: 100,
	}, settings, 0, 0, time.Unix(0, 0))
	engine := &Engine{
		plan: plan, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true,
	}
	_, err := engine.nextMotionSamplesLocked()
	if err == nil || !strings.Contains(err.Error(), "essential points") {
		t.Fatalf("dense frame error = %v, want bounded essential-point rejection", err)
	}
}

// Adaptive framing has to track the plan inside the wire tolerance and must not
// leave the device parked on one rounded percent for a meaningful share of the
// cycle, even with the area focus compressed to a ten-point window.
//
// This asserted "adaptive beats fixed framing by 10%" until the stalling
// patterns were retired. That margin was not coming from the framer: fixed-frame
// rounding only accumulates stationary time when the curve crawls, so the
// baseline it beat was manufactured almost entirely by the very patterns the
// catalog no longer contains. With none of them left, fixed-frame stationary time
// across the whole catalog collapses to near zero and the comparison has nothing
// to measure. The error bound below was always the substantive guarantee.
func TestAdaptiveCatalogFramesTrackThePlanWithoutStairSteps(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	var playedMillis int64
	var adaptiveStationaryMillis int64
	for _, definition := range BuiltinPatternDefinitions() {
		// Curated entries are exact user curves that deliberately hold position.
		if UsesExactImportedCurve(definition) {
			continue
		}
		plan := NewMotionPlan("catalog", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
			AreaFocus: &AreaFocus{MinPercent: 45, MaxPercent: 55},
		}, settings, 0, 0, time.Unix(0, 0))
		engine := &Engine{
			plan: plan, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
			preservePlanKnots: true,
		}
		var samples []MotionSample
		for engine.nextSampleMillis < plan.PeriodMillis*2 {
			chunk, err := engine.nextMotionSamplesLocked()
			if err != nil {
				t.Fatalf("%s: %v", definition.Name, err)
			}
			samples = append(samples, chunk...)
		}
		for index := 1; index < len(samples); index++ {
			if samples[index].TimeMillis <= samples[index-1].TimeMillis {
				t.Fatalf("%s output times are not strictly increasing", definition.Name)
			}
			if math.Round(samples[index].PositionPercent) == math.Round(samples[index-1].PositionPercent) {
				adaptiveStationaryMillis += samples[index].TimeMillis - samples[index-1].TimeMillis
			}
		}
		for at := samples[0].TimeMillis; at <= samples[len(samples)-1].TimeMillis; at += 5 {
			got := interpolateMotionSamples(samples, at)
			want := plan.SampleAt(at).PositionPercent
			if delta := math.Abs(got - want); delta > wireApproximationTolerance+0.05 {
				t.Fatalf("%s adaptive error at %d = %.3f%%", definition.Name, at, delta)
			}
		}

		playedMillis += samples[len(samples)-1].TimeMillis - samples[0].TimeMillis
	}
	if share := float64(adaptiveStationaryMillis) / float64(playedMillis); share > 0.10 {
		t.Fatalf(
			"adaptive framing held a rounded percent for %dms of %dms played (%.1f%%), want under 10%%",
			adaptiveStationaryMillis, playedMillis, share*100,
		)
	}
}

func TestWholePercentSamplingReducesRoundedPlateausWithinWireErrorBound(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	var semanticStationaryMillis int64
	var quantizedStationaryMillis int64
	var fixedStationaryMillis int64
	var semanticPointCount int
	var quantizedPointCount int
	var fixedPointCount int
	var semanticDuplicateEdges int
	var quantizedDuplicateEdges int
	var fixedDuplicateEdges int
	maximumWireError := 0.0
	worstPattern := ""
	for _, definition := range BuiltinPatternDefinitions() {
		if UsesExactImportedCurve(definition) {
			continue
		}
		plan := NewMotionPlan("catalog", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
			AreaFocus: &AreaFocus{MinPercent: 45, MaxPercent: 55},
		}, settings, 0, 0, time.Unix(0, 0))
		semantic := catalogSamples(t, plan, 0)
		quantized := catalogSamples(t, plan, 1)
		fixed := fixedCatalogSamples(plan)
		semanticPointCount += len(semantic)
		quantizedPointCount += len(quantized)
		fixedPointCount += len(fixed)
		semanticStationaryMillis += roundedStationaryMillis(semantic, 1)
		quantizedStationaryMillis += roundedStationaryMillis(quantized, 1)
		fixedStationaryMillis += roundedStationaryMillis(fixed, 1)
		semanticDuplicateEdges += roundedDuplicateEdges(semantic, 1)
		quantizedDuplicateEdges += roundedDuplicateEdges(quantized, 1)
		fixedDuplicateEdges += roundedDuplicateEdges(fixed, 1)
		for at := quantized[0].TimeMillis; at <= quantized[len(quantized)-1].TimeMillis; at += 5 {
			got := interpolateQuantizedMotionSamples(quantized, at, 1)
			want := plan.SampleAt(at).PositionPercent
			delta := math.Abs(got - want)
			if delta > maximumWireError {
				maximumWireError = delta
				worstPattern = definition.Name
			}
			// A whole-percent owner cannot assert sub-step fidelity, so the
			// bound is one full wire step. Sharper reversal ramps concentrate
			// curvature near the apex and cost a fraction of a step there;
			// they remove most of the rounded stationary time in exchange.
			if delta > wireApproximationTolerance+0.7 {
				t.Fatalf("%s whole-percent wire error at %d = %.3f%%", definition.Name, at, delta)
			}
		}
	}
	t.Logf(
		"whole-percent fit: fixed %d points/%d duplicates/%dms stationary; adaptive %d/%d/%dms; fitted %d/%d/%dms; maximum error %.3f%% (%s)",
		fixedPointCount, fixedDuplicateEdges, fixedStationaryMillis,
		semanticPointCount, semanticDuplicateEdges, semanticStationaryMillis,
		quantizedPointCount, quantizedDuplicateEdges, quantizedStationaryMillis,
		maximumWireError, worstPattern,
	)
	if quantizedStationaryMillis >= semanticStationaryMillis*3/4 {
		t.Fatalf(
			"whole-percent stationary time = %dms, want at least 25%% below semantic-frame rounding %dms",
			quantizedStationaryMillis, semanticStationaryMillis,
		)
	}
}

func TestWholePercentPatternChunksDoNotEndAtFixedSamplerWindows(t *testing.T) {
	definition, found := BuiltinPatternDefinition(PatternSteadyDrift)
	if !found {
		t.Fatal("Steady Drift is missing from the default catalog")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	plan := NewMotionPlan("steady-drift", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 20,
	}, settings, 0, 0, time.Unix(0, 0))
	engine := &Engine{
		plan: plan, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true, positionResolutionPercent: 1,
		maximumChunkPoints: maximumAdaptiveChunkPoints,
	}

	for chunkIndex := 0; chunkIndex < 8; chunkIndex++ {
		chunkStart := engine.nextSampleMillis
		nominalTail := chunkStart + int64(defaultChunkSize)*defaultSampleInterval.Milliseconds()
		samples, err := engine.nextMotionSamplesLocked()
		if err != nil {
			t.Fatal(err)
		}
		tail := samples[len(samples)-1].TimeMillis
		if tail < nominalTail {
			t.Fatalf("chunk %d tail = %dms, want fitted coverage through at least %dms", chunkIndex, tail, nominalTail)
		}
		fixedTail := nominalTail - bufferedProbeIntervalMillis
		if tail == fixedTail {
			t.Fatalf("chunk %d retained unrelated fixed sampler-window tail %dms", chunkIndex, tail)
		}
	}
}

func TestRetargetTransitionIsContinuousAndChains(t *testing.T) {
	definition := PatternDefinition{
		ID: "constant", Name: "Constant", Kind: PatternKindRoutine, CycleMillis: 6600,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 50},
			{TimeMillis: 6600, PositionPercent: 50},
		},
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	previous := NewMotionPlan("previous", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
	}, settings, 0, 0, time.Unix(0, 0))
	const handoff = int64(1000)
	next := previous.Retarget("next", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
		AreaFocus: &AreaFocus{MinPercent: 0, MaxPercent: 20},
	}, settings, handoff, time.Unix(1, 0))
	if !transitionRequired(previous, nil, next, handoff) {
		t.Fatal("large focus change did not request a continuity transition")
	}
	transition := newPlanTransition(previous, nil, handoff)
	if got := sampleMotionPath(next, transition, handoff).PositionPercent; got != 50 {
		t.Fatalf("transition start = %.3f, want previous position 50", got)
	}
	if got := sampleMotionPath(next, transition, handoff+retargetTransitionMillis).PositionPercent; got != 10 {
		t.Fatalf("transition end = %.3f, want next position 10", got)
	}
	last := sampleMotionPath(next, transition, handoff).PositionPercent
	for at := handoff + 125; at <= handoff+retargetTransitionMillis; at += 125 {
		position := sampleMotionPath(next, transition, at).PositionPercent
		if delta := math.Abs(position - last); delta >= 15 {
			t.Fatalf("transition step at %d = %.3f%%, want below 15%%", at, delta)
		}
		last = position
	}

	const secondHandoff = handoff + 250
	third := next.Retarget("third", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
		AreaFocus: &AreaFocus{MinPercent: 80, MaxPercent: 100},
	}, settings, secondHandoff, time.Unix(2, 0))
	before := sampleMotionPath(next, transition, secondHandoff).PositionPercent
	chained := newPlanTransition(next, transition, secondHandoff)
	after := sampleMotionPath(third, chained, secondHandoff).PositionPercent
	if math.Abs(after-before) > 0.001 {
		t.Fatalf("repeated retarget snapped from %.3f to %.3f", before, after)
	}
}

func TestPhasePreservingTempoRetargetDoesNotBlendDivergingTimelines(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	definition, found := BuiltinPatternDefinition(PatternCrosscut)
	if !found {
		t.Fatal("Crosscut pattern is missing")
	}
	previous := NewMotionPlan("previous", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 75,
	}, settings, 0, 0, time.Unix(0, 0))
	const handoff = int64(5473)
	next := previous.Retarget("next", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 50,
	}, settings, handoff, time.Unix(1, 0))

	handoffGap := math.Abs(
		previous.SampleAt(handoff).PositionPercent - next.SampleAt(handoff).PositionPercent,
	)
	if handoffGap > transitionPositionEpsilon {
		t.Fatalf("handoff gap = %.4f%%, test requires a continuous handoff", handoffGap)
	}
	pathGap := math.Abs(
		previous.SampleAt(handoff+500).PositionPercent - next.SampleAt(handoff+500).PositionPercent,
	)
	if pathGap < 3 {
		t.Fatalf("path gap = %.3f%%, test does not exercise diverging tempos", pathGap)
	}
	if transitionRequired(previous, nil, next, handoff) {
		t.Fatal("continuous speed-only handoff requested a moving-path crossfade")
	}
}

func TestPhasePreservingTempoRetargetBridgesDirectionMismatchNearReversal(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	definition, found := BuiltinPatternDefinition(PatternStroke)
	if !found {
		t.Fatal("Stroke pattern is missing")
	}
	previous := NewMotionPlan("previous", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 10,
	}, settings, 0, 0, time.Unix(0, 0))
	var (
		handoff       int64
		next          MotionPlan
		foundMismatch bool
	)
	for candidate := int64(0); candidate < previous.PeriodMillis; candidate += 5 {
		candidatePlan := previous.Retarget("next", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: 20,
		}, settings, candidate, time.Unix(1, 0))
		beforeDirection := previous.DirectionAt(candidate)
		afterDirection := candidatePlan.DirectionAt(candidate)
		if beforeDirection != 0 && afterDirection != 0 && beforeDirection != afterDirection {
			handoff = candidate
			next = candidatePlan
			foundMismatch = true
			break
		}
	}
	if !foundMismatch {
		t.Fatal("could not find a speed-retarget reversal-guide mismatch")
	}

	handoffGap := math.Abs(
		previous.SampleAt(handoff).PositionPercent - next.SampleAt(handoff).PositionPercent,
	)
	if handoffGap > transitionPositionEpsilon {
		t.Fatalf("handoff gap = %.4f%%, test requires a position-continuous handoff", handoffGap)
	}
	beforeDirection := previous.DirectionAt(handoff)
	afterDirection := next.DirectionAt(handoff)
	if beforeDirection == afterDirection {
		t.Fatalf("directions = %d and %d, test requires a reversal-guide mismatch", beforeDirection, afterDirection)
	}
	if !transitionRequired(previous, nil, next, handoff) {
		t.Fatal("opposed reversal directions skipped the bounded transition")
	}
}

func TestTransitionHistoryExpiresByPlaybackTimeNotBufferedTail(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	previous := NewMotionPlan("previous", MotionTarget{
		PatternID: PatternStroke, SpeedPercent: 100,
	}, settings, 0, 0, time.Unix(0, 0))
	const handoff = int64(1000)
	next := previous.Retarget("next", MotionTarget{
		PatternID: PatternPulse, SpeedPercent: 100,
	}, settings, handoff, time.Unix(1, 0))
	engine := &Engine{
		plan: next, transition: newPlanTransition(previous, nil, handoff), nextSampleMillis: handoff,
		chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true,
	}
	if _, err := engine.nextMotionSamplesLocked(); err != nil {
		t.Fatal(err)
	}
	if engine.transition == nil {
		t.Fatal("future buffer generation discarded transition before playback reached it")
	}
	if active := pruneTransitionHistory(engine.transition, handoff+retargetTransitionMillis-1); active == nil {
		t.Fatal("transition expired before its playback endpoint")
	}
	if active := pruneTransitionHistory(engine.transition, handoff+retargetTransitionMillis); active != nil {
		t.Fatal("transition remained active at its playback endpoint")
	}
}

func TestExpiredTransitionHistoryDoesNotAlterLaterPatternFrames(t *testing.T) {
	curve, err := NewCurve([]CurvePoint{
		{TimeMillis: 0, PositionPercent: 20},
		{TimeMillis: 100, PositionPercent: 21},
		{TimeMillis: 200, PositionPercent: 20},
		{TimeMillis: 1000, PositionPercent: 20},
	}, 1000, true)
	if err != nil {
		t.Fatal(err)
	}
	plan := MotionPlan{
		ID: "subtle", PeriodMillis: 1000, Loop: true, curve: curve,
	}
	previous := plan
	previous.ID = "previous"
	withHistory := &Engine{
		plan: plan, transition: newPlanTransition(previous, nil, 0), nextSampleMillis: 1000,
		chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true,
	}
	withoutHistory := &Engine{
		plan: plan, nextSampleMillis: 1000,
		chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true,
	}
	got, err := withHistory.nextMotionSamplesLocked()
	if err != nil {
		t.Fatal(err)
	}
	want, err := withoutHistory.nextMotionSamplesLocked()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("expired history emitted %d samples, want %d", len(got), len(want))
	}
	for index := range got {
		if got[index].TimeMillis != want[index].TimeMillis ||
			math.Abs(got[index].PositionPercent-want[index].PositionPercent) > 0.0001 {
			t.Fatalf("expired history sample %d = %+v, want %+v", index, got[index], want[index])
		}
	}
}

func TestRetargetFromStateChoosesPhaseFromEffectivePath(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	previous := NewMotionPlan("previous", MotionTarget{
		PatternID: PatternStroke, SpeedPercent: 100,
	}, settings, 0, 0, time.Unix(0, 0))
	targetDefinition := PatternDefinition{
		ID: "triangle", Name: "Triangle", Kind: PatternKindRoutine, CycleMillis: 6600,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 3300, PositionPercent: 100},
			{TimeMillis: 6600, PositionPercent: 0},
		},
	}
	const handoff = int64(2000)
	next := previous.retargetFromState("next", MotionTarget{
		PatternID: targetDefinition.ID, Pattern: &targetDefinition, SpeedPercent: 100,
	}, settings, handoff, 87, 1, 0, time.Unix(2, 0))
	position := next.SampleAt(handoff).PositionPercent
	if math.Abs(position-87) > 3 {
		t.Fatalf("retarget handoff position = %.3f, want near effective path position 87", position)
	}
	if direction := next.DirectionAt(handoff); direction < 0 {
		t.Fatalf("retarget handoff direction = %d, want supplied positive direction preserved", direction)
	}
}

func TestCatalogRetargetTransitionsDoNotCreateRapidChatter(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	definitions := BuiltinPatternDefinitions()
	const handoff = int64(2200)
	for _, previousDefinition := range definitions {
		if UsesExactImportedCurve(previousDefinition) {
			continue
		}
		previous := NewMotionPlan("previous", MotionTarget{
			PatternID: previousDefinition.ID, Pattern: &previousDefinition, SpeedPercent: 100,
		}, settings, 0, 0, time.Unix(0, 0))
		position := previous.SampleAt(handoff).PositionPercent
		direction := previous.DirectionAt(handoff)
		for _, nextDefinition := range definitions {
			if nextDefinition.ID == previousDefinition.ID || UsesExactImportedCurve(nextDefinition) {
				continue
			}
			next := previous.retargetFromState("next", MotionTarget{
				PatternID: nextDefinition.ID, Pattern: &nextDefinition, SpeedPercent: 100,
			}, settings, handoff, position, direction, previous.VelocityAt(handoff), time.Unix(1, 0))
			assertTransitionFrameHasNoRapidChatter(
				t, previousDefinition.Name+" -> "+nextDefinition.Name, previous, next, handoff,
			)
		}
	}
}

func TestCatalogFocusTransitionsDoNotCreateRapidChatter(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	const handoff = int64(2200)
	for _, definition := range BuiltinPatternDefinitions() {
		if UsesExactImportedCurve(definition) {
			continue
		}
		full := NewMotionPlan("full", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
		}, settings, 0, 0, time.Unix(0, 0))
		focusedTarget := MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
			AreaFocus: &AreaFocus{MinPercent: 45, MaxPercent: 55},
		}
		focused := full.retargetFromState(
			"focused", focusedTarget, settings, handoff,
			full.SampleAt(handoff).PositionPercent, full.DirectionAt(handoff), full.VelocityAt(handoff), time.Unix(1, 0),
		)
		assertTransitionFrameHasNoRapidChatter(t, definition.Name+" full -> focus", full, focused, handoff)

		focusedStart := NewMotionPlan("focused-start", focusedTarget, settings, 0, 0, time.Unix(0, 0))
		unfocused := focusedStart.retargetFromState(
			"unfocused", MotionTarget{
				PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
			}, settings, handoff,
			focusedStart.SampleAt(handoff).PositionPercent,
			focusedStart.DirectionAt(handoff), focusedStart.VelocityAt(handoff), time.Unix(1, 0),
		)
		assertTransitionFrameHasNoRapidChatter(t, definition.Name+" focus -> full", focusedStart, unfocused, handoff)
	}
}

func assertTransitionFrameHasNoRapidChatter(t *testing.T, label string, previous, next MotionPlan, handoff int64) {
	t.Helper()
	assertTransitionFrameAtResolution(t, label+" semantic", previous, next, handoff, 0)
	assertTransitionFrameAtResolution(t, label+" Cloud", previous, next, handoff, 1)
}

func assertTransitionFrameAtResolution(
	t *testing.T,
	label string,
	previous MotionPlan,
	next MotionPlan,
	handoff int64,
	resolution float64,
) {
	t.Helper()
	engine := &Engine{
		plan: next, transition: newPlanTransition(previous, nil, handoff), nextSampleMillis: handoff,
		chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true, positionResolutionPercent: resolution,
	}
	samples, err := engine.nextMotionSamplesLocked()
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	following, err := engine.nextMotionSamplesLocked()
	if err != nil {
		t.Fatalf("%s follow-up: %v", label, err)
	}
	samples = append(samples, following...)
	start, ok := motionSampleAt(samples, handoff)
	if !ok || math.Abs(start.PositionPercent-previous.SampleAt(handoff).PositionPercent) > 0.0001 {
		t.Fatalf("%s transition start = %+v, want exact previous-path handoff", label, start)
	}
	endMillis := handoff + retargetTransitionMillis
	end, ok := motionSampleAt(samples, endMillis)
	if !ok || math.Abs(end.PositionPercent-next.SampleAt(endMillis).PositionPercent) > 0.0001 {
		t.Fatalf("%s transition end = %+v, want exact next-path handoff", label, end)
	}
	points := make([]CurvePoint, 0, len(samples))
	for _, sample := range samples {
		points = append(points, CurvePoint{
			TimeMillis: sample.TimeMillis - handoff, PositionPercent: sample.PositionPercent,
		})
	}
	anchors := curveReversalAnchors(points)
	for index := 1; index < len(anchors)-1; index++ {
		left := points[anchors[index-1]]
		current := points[anchors[index]]
		right := points[anchors[index+1]]
		if current.TimeMillis > retargetTransitionMillis {
			continue
		}
		prominence := math.Min(
			math.Abs(current.PositionPercent-left.PositionPercent),
			math.Abs(current.PositionPercent-right.PositionPercent),
		)
		flank := min(current.TimeMillis-left.TimeMillis, right.TimeMillis-current.TimeMillis)
		if prominence <= MinimumPatternReversalProminence && flank <= maximumPatternChatterFlankMillis {
			t.Fatalf(
				"%s transition introduced %.3f%% reversal with %dms flank: left=%+v current=%+v right=%+v samples=%+v",
				label, prominence, flank, left, current, right, points,
			)
		}
	}
}

func motionSampleAt(samples []MotionSample, at int64) (MotionSample, bool) {
	for _, sample := range samples {
		if sample.TimeMillis == at {
			return sample, true
		}
	}
	return MotionSample{}, false
}

func pointTimes(points []transport.TimedPoint) []int64 {
	times := make([]int64, len(points))
	for index, point := range points {
		times[index] = point.TimeMillis
	}
	return times
}

func containsMillis(values []int64, want int64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func interpolateMotionSamples(samples []MotionSample, at int64) float64 {
	for index := 1; index < len(samples); index++ {
		if samples[index].TimeMillis < at {
			continue
		}
		left, right := samples[index-1], samples[index]
		fraction := float64(at-left.TimeMillis) / float64(right.TimeMillis-left.TimeMillis)
		return left.PositionPercent + (right.PositionPercent-left.PositionPercent)*fraction
	}
	return samples[len(samples)-1].PositionPercent
}

func catalogSamples(t *testing.T, plan MotionPlan, resolution float64) []MotionSample {
	t.Helper()
	maximumPoints := maximumAdaptiveChunkPoints
	if resolution >= 1 {
		// Match the API v3 HSP add limit advertised by CloudRESTTransport.
		maximumPoints = 100
	}
	engine := &Engine{
		plan: plan, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true, positionResolutionPercent: resolution,
		maximumChunkPoints: maximumPoints,
	}
	var samples []MotionSample
	for engine.nextSampleMillis < plan.PeriodMillis*2 {
		chunk, err := engine.nextMotionSamplesLocked()
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, chunk...)
	}
	return samples
}

func fixedCatalogSamples(plan MotionPlan) []MotionSample {
	end := plan.PeriodMillis * 2
	samples := make([]MotionSample, 0, end/defaultSampleInterval.Milliseconds()+2)
	for at := int64(0); at < end; at += defaultSampleInterval.Milliseconds() {
		samples = append(samples, plan.SampleAt(at))
	}
	samples = append(samples, plan.SampleAt(end))
	return samples
}

func roundedStationaryMillis(samples []MotionSample, resolution float64) int64 {
	var stationary int64
	for index := 1; index < len(samples); index++ {
		if quantizedMotionPosition(samples[index-1].PositionPercent, resolution) ==
			quantizedMotionPosition(samples[index].PositionPercent, resolution) {
			stationary += samples[index].TimeMillis - samples[index-1].TimeMillis
		}
	}
	return stationary
}

func roundedDuplicateEdges(samples []MotionSample, resolution float64) int {
	duplicates := 0
	for index := 1; index < len(samples); index++ {
		if quantizedMotionPosition(samples[index-1].PositionPercent, resolution) ==
			quantizedMotionPosition(samples[index].PositionPercent, resolution) {
			duplicates++
		}
	}
	return duplicates
}

func interpolateQuantizedMotionSamples(samples []MotionSample, at int64, resolution float64) float64 {
	for index := 1; index < len(samples); index++ {
		if samples[index].TimeMillis < at {
			continue
		}
		left, right := samples[index-1], samples[index]
		fraction := float64(at-left.TimeMillis) / float64(right.TimeMillis-left.TimeMillis)
		leftPosition := quantizedMotionPosition(left.PositionPercent, resolution)
		rightPosition := quantizedMotionPosition(right.PositionPercent, resolution)
		return leftPosition + (rightPosition-leftPosition)*fraction
	}
	return quantizedMotionPosition(samples[len(samples)-1].PositionPercent, resolution)
}

type samplingCapabilityTransport struct {
	*transport.Fake
	resolution        float64
	afterStrokeWindow bool
	maximumPoints     int
}

func (t *samplingCapabilityTransport) MotionSamplingCapabilities() transport.MotionSamplingCapabilities {
	return transport.MotionSamplingCapabilities{
		PositionResolutionPercent:   t.resolution,
		ResolutionAfterStrokeWindow: t.afterStrokeWindow,
		MaximumPointsPerAppend:      t.maximumPoints,
	}
}
