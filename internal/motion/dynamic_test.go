package motion

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestDynamicAnchorLoopPassesThroughInteriorAnchors(t *testing.T) {
	definition := DynamicDefinition{Anchors: []DynamicAnchor{
		{Name: "base", PositionPercent: 8},
		{Name: "middle", PositionPercent: 50},
		{Name: "tip", PositionPercent: 92},
	}}
	content := dynamicContent(definition)
	curve, err := content.buildCurve(neutralPlaybackScale())
	if err != nil {
		t.Fatalf("build dynamic curve: %v", err)
	}
	if len(content.points) != 5 {
		t.Fatalf("dynamic points = %v, want forward/reverse traversal without duplicated reversals", content.points)
	}
	if velocity := curve.Velocity(content.points[1].TimeMillis); velocity <= 1 {
		t.Fatalf("first interior anchor velocity = %.3f, want positive pass-through", velocity)
	}
	if velocity := curve.Velocity(content.points[3].TimeMillis); velocity >= -1 {
		t.Fatalf("return interior anchor velocity = %.3f, want negative pass-through", velocity)
	}
}

func TestDynamicPlanKeepsSlowReversalStallsBrief(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	definition := NormalizeDynamicDefinition(DynamicDefinition{CenterPercent: 50, SpanPercent: 24})
	plan := NewMotionPlan("dynamic", MotionTarget{Dynamic: &definition, SpeedPercent: 20}, settings, 0, 0, time.Unix(0, 0))
	longestLowVelocity := int64(0)
	currentLowVelocity := int64(0)
	for at := int64(0); at <= plan.PeriodMillis; at += 10 {
		if math.Abs(plan.VelocityAt(at)) < 1 {
			currentLowVelocity += 10
			longestLowVelocity = max(longestLowVelocity, currentLowVelocity)
		} else {
			currentLowVelocity = 0
		}
	}
	if longestLowVelocity > 200 {
		t.Fatalf("slow dynamic loop low-velocity window = %dms, want <= 200ms", longestLowVelocity)
	}
}

func TestDynamicRetargetChoosesPositionAndVelocityCompatiblePhase(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	first := NormalizeDynamicDefinition(DynamicDefinition{CenterPercent: 45, SpanPercent: 70})
	second := NormalizeDynamicDefinition(DynamicDefinition{Anchors: []DynamicAnchor{
		{Name: "base", PositionPercent: 8},
		{Name: "upper", PositionPercent: 72},
		{Name: "tip", PositionPercent: 92},
	}})
	previous := NewMotionPlan("first", MotionTarget{Dynamic: &first, SpeedPercent: 35}, settings, 0, 0, time.Unix(0, 0))
	for _, phase := range []float64{0.13, 0.31, 0.62, 0.84} {
		at := int64(float64(previous.PeriodMillis) * phase)
		next := previous.Retarget("second", MotionTarget{Dynamic: &second, SpeedPercent: 35}, settings, at, time.Unix(1, 0))
		positionJump := math.Abs(next.SampleAt(at).PositionPercent - previous.SampleAt(at).PositionPercent)
		if positionJump > 5 {
			t.Fatalf("phase %.2f retarget position jump = %.2f, want <= 5", phase, positionJump)
		}
		left, right := previous.DirectionAt(at), next.DirectionAt(at)
		if left != 0 && right != 0 && left != right {
			t.Fatalf("phase %.2f direction changed from %d to %d", phase, left, right)
		}
	}
}

func TestDynamicVariationIsBoundedAndLoopClosed(t *testing.T) {
	content := dynamicContent(DynamicDefinition{CenterPercent: 50, SpanPercent: 70, VariationPercent: 100})
	if first, last := content.points[0], content.points[len(content.points)-1]; first.PositionPercent != last.PositionPercent {
		t.Fatalf("dynamic variation loop does not close: first=%v last=%v", first, last)
	}
	for _, point := range content.points {
		if point.PositionPercent < 0 || point.PositionPercent > 100 {
			t.Fatalf("dynamic variation point escaped semantic travel: %+v", point)
		}
	}
}

func TestDynamicFullSpanVariationAlwaysCompiles(t *testing.T) {
	models := []string{
		config.HandyModelOriginal,
		config.HandyModel2Standard,
		config.HandyModel2Pro,
	}
	for variation := 0; variation <= 100; variation++ {
		definition := NormalizeDynamicDefinition(DynamicDefinition{
			CenterPercent: 50, SpanPercent: 100, VariationPercent: variation,
		})
		content := dynamicContent(definition)
		for index, point := range content.points {
			if point.PositionPercent < 0 || point.PositionPercent > 100 ||
				math.IsNaN(point.PositionPercent) || math.IsInf(point.PositionPercent, 0) {
				t.Fatalf("variation=%d point=%d escaped bounds: %.17g", variation, index, point.PositionPercent)
			}
		}
		for _, model := range models {
			settings := config.DefaultSettings().Motion
			settings.SpeedMinPercent = 1
			settings.SpeedMaxPercent = 100
			settings.HandyModel = model
			plan := NewMotionPlan("full-span", MotionTarget{
				Dynamic: &definition, SpeedPercent: 68,
			}, settings, 0, 0, time.Unix(0, 0))
			if err := plan.compilationError(); err != nil {
				t.Fatalf("variation=%d model=%s: %v", variation, model, err)
			}
			if len(plan.curve.points) < 2 {
				t.Fatalf("variation=%d model=%s compiled %d points", variation, model, len(plan.curve.points))
			}
			for _, at := range []int64{0, plan.PeriodMillis / 2, plan.PeriodMillis} {
				position := plan.SampleAt(at).PositionPercent
				if position < 0 || position > 100 || math.IsNaN(position) || math.IsInf(position, 0) {
					t.Fatalf("variation=%d model=%s sample=%d position=%.17g", variation, model, at, position)
				}
			}
		}
	}
}

func TestDynamicVariationUsesLongDeterministicOrganicPhrase(t *testing.T) {
	definition := DynamicDefinition{CenterPercent: 50, SpanPercent: 70, VariationPercent: 60}
	first := dynamicContent(definition)
	second := dynamicContent(definition)
	if !reflect.DeepEqual(first.points, second.points) {
		t.Fatal("dynamic organic phrase is not deterministic")
	}
	if len(first.points) <= 25 {
		t.Fatalf("dynamic variation has %d points, want a phrase longer than the former four-cycle motif", len(first.points))
	}

	minimumRatio := math.MaxFloat64
	maximumRatio := 0.0
	for index := 1; index < len(first.points); index++ {
		distance := math.Abs(first.points[index].PositionPercent - first.points[index-1].PositionPercent)
		if distance < 1 {
			continue
		}
		ratio := float64(first.points[index].TimeMillis-first.points[index-1].TimeMillis) / distance
		minimumRatio = math.Min(minimumRatio, ratio)
		maximumRatio = math.Max(maximumRatio, ratio)
	}
	if maximumRatio/minimumRatio < 1.10 {
		t.Fatalf("dynamic leg timing ratio %.3f..%.3f is still metronomic", minimumRatio, maximumRatio)
	}
	if maximumRatio/minimumRatio > 1.70 {
		t.Fatalf("dynamic leg timing ratio %.3f..%.3f exceeds the bounded texture", minimumRatio, maximumRatio)
	}

	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		for _, span := range []int{20, 40, 70, 100} {
			settings := config.DefaultSettings().Motion
			settings.SpeedMaxPercent = 100
			settings.HandyModel = model
			normalized := NormalizeDynamicDefinition(DynamicDefinition{
				CenterPercent: 50, SpanPercent: span, VariationPercent: 100,
			})
			plan := NewMotionPlan("organic", MotionTarget{Dynamic: &normalized, SpeedPercent: 100}, settings, 0, 0, time.Unix(0, 0))
			if plan.PeriodMillis < int64(minimumDynamicVariationLoopSeconds*1000) {
				t.Errorf("model=%s span=%d variation repeats in %dms, want at least %.0fs at maximum speed",
					model, span, plan.PeriodMillis, minimumDynamicVariationLoopSeconds)
			}
		}
	}
}

func TestDynamicPlaybackHonorsRuntimeEnvelope(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	for _, span := range []int{20, 40, 70, 100} {
		for _, variation := range []int{0, 30, 100} {
			for _, speed := range []int{1, 20, 73, 100} {
				definition := NormalizeDynamicDefinition(DynamicDefinition{
					CenterPercent: 50, SpanPercent: span, VariationPercent: variation,
				})
				plan := NewMotionPlan("bounded", MotionTarget{
					Dynamic: &definition, SpeedPercent: speed,
				}, settings, 0, 0, time.Unix(0, 0))
				if acceleration := maximumPlanAcceleration(plan); acceleration > runtimeMaxAccelerationPercentPerSecond2*1.002 {
					t.Errorf("span=%d variation=%d speed=%d acceleration=%.1f, over %.1f",
						span, variation, speed, acceleration, runtimeMaxAccelerationPercentPerSecond2)
				}
				if gap := reversalGap(plan.curve.authoredKnots, plan.curve.duration, true); gap > 0 {
					playedGap := float64(gap) * float64(plan.PeriodMillis) / float64(plan.curve.duration)
					if playedGap+0.01 < float64(runtimeMinimumReversalGapMillis) {
						t.Errorf("span=%d variation=%d speed=%d reversal gap=%.1fms, below %dms",
							span, variation, speed, playedGap, runtimeMinimumReversalGapMillis)
					}
				}
			}
		}
	}
}

func TestNormalizeDynamicSpanEnvelope(t *testing.T) {
	legacy := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 80, VariationPercent: 30,
	})
	if legacy.SpanProfile != "" || legacy.SpanMinPercent != 0 || legacy.PhraseSeed != 0 {
		t.Fatalf("legacy definition gained an explicit envelope: %+v", legacy)
	}

	steady := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 80, SpanMinPercent: 30,
		SpanProfile: DynamicSpanProfileSteady,
	})
	if steady.SpanProfile != DynamicSpanProfileSteady || steady.SpanMinPercent != 80 || steady.PhraseSeed != 0 {
		t.Fatalf("steady envelope = %+v", steady)
	}

	wander := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 80, SpanMinPercent: 28,
		SpanProfile: " WANDER ",
	})
	if wander.SpanProfile != DynamicSpanProfileWander || wander.SpanMinPercent != 28 || wander.PhraseSeed == 0 {
		t.Fatalf("wander envelope = %+v", wander)
	}
	if again := NormalizeDynamicDefinition(wander); !reflect.DeepEqual(again, wander) {
		t.Fatalf("span normalization is not idempotent: first=%+v second=%+v", wander, again)
	}

	missingFloor := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 80, SpanProfile: DynamicSpanProfileContrast,
	})
	if missingFloor.SpanProfile != DynamicSpanProfileSteady || missingFloor.SpanMinPercent != 80 {
		t.Fatalf("missing span floor did not fail steady: %+v", missingFloor)
	}

	anchorEnvelope := NormalizeDynamicDefinition(DynamicDefinition{
		Anchors: []DynamicAnchor{
			{Name: "base", PositionPercent: 8},
			{Name: "middle", PositionPercent: 50},
			{Name: "tip", PositionPercent: 92},
		},
		SpanMinPercent: 32, SpanProfile: DynamicSpanProfileBreathe,
	})
	if anchorEnvelope.SpanPercent != 84 || anchorEnvelope.SpanMinPercent != 32 || anchorEnvelope.PhraseSeed == 0 {
		t.Fatalf("anchor envelope = %+v", anchorEnvelope)
	}
}

func TestDynamicSpanProfilesProduceLongBoundedDistinctPhrases(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	settings.HandyModel = config.HandyModel2Pro

	var previous []CurvePoint
	for _, profile := range []string{
		DynamicSpanProfileBreathe,
		DynamicSpanProfileWander,
		DynamicSpanProfileContrast,
	} {
		definition := NormalizeDynamicDefinition(DynamicDefinition{
			CenterPercent: 50, SpanPercent: 82, SpanMinPercent: 28,
			SpanProfile: profile, VariationPercent: 25,
		})
		first := dynamicContent(definition)
		second := dynamicContent(definition)
		if !reflect.DeepEqual(first.points, second.points) {
			t.Fatalf("profile %s is not deterministic", profile)
		}
		if len(first.points) < minimumDynamicSpanEnvelopeCycles*2+1 {
			t.Fatalf("profile %s has only %d points", profile, len(first.points))
		}
		if first.points[0].PositionPercent != first.points[len(first.points)-1].PositionPercent {
			t.Fatalf("profile %s does not close: first=%+v last=%+v", profile, first.points[0], first.points[len(first.points)-1])
		}
		minimumSpan, maximumSpan := dynamicTwoPointCycleSpanBounds(first.points)
		if minimumSpan < 28-1e-9 || maximumSpan > 82+1e-9 {
			t.Fatalf("profile %s escaped 28..82: %.3f..%.3f", profile, minimumSpan, maximumSpan)
		}
		if minimumSpan > 40 || maximumSpan < 70 {
			t.Fatalf("profile %s did not meaningfully explore its range: %.3f..%.3f", profile, minimumSpan, maximumSpan)
		}
		if previous != nil && reflect.DeepEqual(previous, first.points) {
			t.Fatalf("profile %s duplicated the preceding profile", profile)
		}
		previous = first.points

		plan := NewMotionPlan("span-envelope", MotionTarget{
			Dynamic: &definition, SpeedPercent: 100,
		}, settings, 0, 0, time.Unix(0, 0))
		if err := plan.compilationError(); err != nil {
			t.Fatalf("profile %s compilation: %v", profile, err)
		}
		if plan.PeriodMillis < int64(minimumDynamicSpanEnvelopeLoopSeconds*1000) {
			t.Fatalf("profile %s repeats in %dms, want at least %.0fs", profile,
				plan.PeriodMillis, minimumDynamicSpanEnvelopeLoopSeconds)
		}
		if acceleration := maximumPlanAcceleration(plan); acceleration > runtimeMaxAccelerationPercentPerSecond2*1.002 {
			t.Fatalf("profile %s acceleration %.1f exceeds %.1f", profile,
				acceleration, runtimeMaxAccelerationPercentPerSecond2)
		}
	}
}

func TestDynamicEverydayProfilesVaryWithinShortPerceptualWindows(t *testing.T) {
	for _, profile := range []string{DynamicSpanProfileWander, DynamicSpanProfileContrast} {
		definition := NormalizeDynamicDefinition(DynamicDefinition{
			CenterPercent: 46, SpanPercent: 92, SpanMinPercent: 20,
			SpanProfile: profile, PhraseSeed: 1844129920,
			VariationPercent: 25, SegmentSeconds: 30,
		})
		spans := dynamicTwoPointCycleSpans(dynamicContent(definition).points)
		if len(spans) < 24 {
			t.Fatalf("profile %s has only %d cycles", profile, len(spans))
		}
		window := 6
		minimumRange := float64(definition.SpanPercent-definition.SpanMinPercent) * 0.18
		for start := range spans {
			minimum, maximum := math.MaxFloat64, 0.0
			for offset := range window {
				span := spans[(start+offset)%len(spans)]
				minimum = math.Min(minimum, span)
				maximum = math.Max(maximum, span)
			}
			if maximum-minimum < minimumRange {
				t.Fatalf("profile %s cycles %d..%d vary only %.2f%% (%.2f..%.2f); want at least %.2f%%",
					profile, start, start+window-1, maximum-minimum, minimum, maximum, minimumRange)
			}
		}
		shortWindow := 4
		minimumShortRange := float64(definition.SpanPercent-definition.SpanMinPercent) * 0.08
		for start := range spans {
			minimum, maximum := math.MaxFloat64, 0.0
			for offset := range shortWindow {
				span := spans[(start+offset)%len(spans)]
				minimum = math.Min(minimum, span)
				maximum = math.Max(maximum, span)
			}
			if maximum-minimum < minimumShortRange {
				t.Fatalf("profile %s cycles %d..%d form a %.2f%% span plateau; want at least %.2f%%",
					profile, start, start+shortWindow-1, maximum-minimum, minimumShortRange)
			}
		}
	}
}

func TestDynamicReversalsEaseAcrossTheWholeLeg(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 80, SpanMinPercent: 80,
		SpanProfile: DynamicSpanProfileSteady,
	})
	content := dynamicContent(definition)
	plan := NewMotionPlan("whole-leg-easing", MotionTarget{
		Dynamic: &definition, SpeedPercent: 85,
	}, settings, 0, 0, time.Unix(0, 0))
	if err := plan.compilationError(); err != nil {
		t.Fatal(err)
	}
	legEnd := int64(math.Round(
		float64(content.points[1].TimeMillis) / float64(content.duration) * float64(plan.PeriodMillis),
	))
	middleVelocity := math.Abs(dynamicPlayedVelocity(plan, legEnd/2))
	approachVelocity := math.Abs(dynamicPlayedVelocity(plan, legEnd*9/10))
	turnVelocity := math.Abs(plan.curve.velocityFloat(float64(content.points[1].TimeMillis)))
	if middleVelocity <= 0 {
		t.Fatal("dynamic leg has no mid-stroke velocity")
	}
	if ratio := approachVelocity / middleVelocity; ratio > 0.55 {
		t.Fatalf("dynamic reversal retains %.1f%% of mid-stroke velocity at 90%% of the leg; want whole-leg braking",
			ratio*100)
	}
	if turnVelocity > 1e-9 {
		t.Fatalf("dynamic reversal authored velocity = %.9f%%/ms, want zero", turnVelocity)
	}
	if acceleration := maximumPlanAcceleration(plan); acceleration > runtimeMaxAccelerationPercentPerSecond2*1.002 {
		t.Fatalf("dynamic eased acceleration %.1f exceeds %.1f", acceleration, runtimeMaxAccelerationPercentPerSecond2)
	}
}

func TestDynamicWireFrameCarriesEasingToBufferedTransport(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 80, SpanMinPercent: 80,
		SpanProfile: DynamicSpanProfileSteady,
	})
	content := dynamicContent(definition)
	plan := NewMotionPlan("wire-easing", MotionTarget{
		Dynamic: &definition, SpeedPercent: 85,
	}, settings, 0, 0, time.Unix(0, 0))
	legEnd := int64(math.Round(
		float64(content.points[1].TimeMillis) / float64(content.duration) * float64(plan.PeriodMillis),
	))
	engine := &Engine{
		plan: plan, chunkSize: defaultChunkSize, sampleInterval: defaultSampleInterval,
		preservePlanKnots: true, positionResolutionPercent: 1,
		maximumChunkPoints: maximumAdaptiveChunkPoints,
	}
	samples, err := engine.nextMotionSamplesLocked()
	if err != nil {
		t.Fatal(err)
	}
	peakSlope, finalSlope := 0.0, math.MaxFloat64
	for index := 1; index < len(samples); index++ {
		left, right := samples[index-1], samples[index]
		if right.TimeMillis > legEnd || right.TimeMillis <= left.TimeMillis {
			continue
		}
		slope := math.Abs(right.PositionPercent-left.PositionPercent) /
			float64(right.TimeMillis-left.TimeMillis)
		peakSlope = math.Max(peakSlope, slope)
		if right.TimeMillis == legEnd {
			finalSlope = slope
		}
	}
	if peakSlope <= 0 || finalSlope == math.MaxFloat64 {
		t.Fatalf("wire samples do not include the first eased reversal at %dms: %+v", legEnd, samples)
	}
	if ratio := finalSlope / peakSlope; ratio > 0.65 {
		t.Fatalf("final wire segment retains %.1f%% of peak segment velocity; easing was flattened: %+v",
			ratio*100, samples)
	}
}

func dynamicPlayedVelocity(plan MotionPlan, streamMillis int64) float64 {
	phase := plan.PhaseAt(streamMillis)
	authoredMillis := phase * float64(plan.curve.duration)
	return plan.curve.velocityFloat(authoredMillis) *
		float64(plan.curve.duration) / float64(plan.PeriodMillis) * 1000 * plan.focus.gain()
}

func TestDynamicModerateVariationIsLocallyPerceptible(t *testing.T) {
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 46, SpanPercent: 92, SpanMinPercent: 20,
		SpanProfile: DynamicSpanProfileContrast, PhraseSeed: 1844129920,
		VariationPercent: 25, SegmentSeconds: 30,
	})
	points := dynamicContent(definition).points
	const window = 16
	for start := 1; start+window < len(points); start += window {
		minimumRatio, maximumRatio := math.MaxFloat64, 0.0
		for index := start; index < start+window; index++ {
			distance := math.Abs(points[index].PositionPercent - points[index-1].PositionPercent)
			if distance < MinimumDynamicSpanPercent {
				continue
			}
			ratio := float64(points[index].TimeMillis-points[index-1].TimeMillis) / distance
			minimumRatio = math.Min(minimumRatio, ratio)
			maximumRatio = math.Max(maximumRatio, ratio)
		}
		if minimumRatio < math.MaxFloat64 && maximumRatio/minimumRatio < 1.10 {
			t.Fatalf("legs %d..%d timing ratio %.3f is locally metronomic", start, start+window-1,
				maximumRatio/minimumRatio)
		}
	}

	steady := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 40, SpanMinPercent: 40,
		SpanProfile: DynamicSpanProfileSteady, VariationPercent: 25,
	})
	centers := dynamicTwoPointCycleCenters(dynamicContent(steady).points)
	minimum, maximum := math.MaxFloat64, 0.0
	for _, center := range centers {
		minimum = math.Min(minimum, center)
		maximum = math.Max(maximum, center)
	}
	if maximum-minimum < 8 {
		t.Fatalf("25%% variation moves the center only %.2f%% (%.2f..%.2f), want perceptible drift",
			maximum-minimum, minimum, maximum)
	}
}

func TestDynamicExplicitSteadySpanIsIndependentFromVariation(t *testing.T) {
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 70, SpanMinPercent: 25,
		SpanProfile: DynamicSpanProfileSteady, VariationPercent: 100,
	})
	content := dynamicContent(definition)
	minimumSpan, maximumSpan := dynamicTwoPointCycleSpanBounds(content.points)
	if math.Abs(minimumSpan-70) > 1e-9 || math.Abs(maximumSpan-70) > 1e-9 {
		t.Fatalf("steady span changed under center/rhythm variation: %.6f..%.6f", minimumSpan, maximumSpan)
	}
}

func TestDynamicSpanEnvelopeCompilesAcrossProfilesAndHandyModels(t *testing.T) {
	for _, profile := range []string{
		DynamicSpanProfileBreathe,
		DynamicSpanProfileWander,
		DynamicSpanProfileContrast,
	} {
		for _, span := range []int{40, 70, 100} {
			for _, variation := range []int{0, 100} {
				definition := NormalizeDynamicDefinition(DynamicDefinition{
					CenterPercent: 50, SpanPercent: span, SpanMinPercent: 20,
					SpanProfile: profile, VariationPercent: variation,
				})
				for _, model := range []string{
					config.HandyModelOriginal,
					config.HandyModel2Standard,
					config.HandyModel2Pro,
				} {
					settings := config.DefaultSettings().Motion
					settings.SpeedMinPercent = 1
					settings.SpeedMaxPercent = 100
					settings.HandyModel = model
					plan := NewMotionPlan("span-envelope-matrix", MotionTarget{
						Dynamic: &definition, SpeedPercent: 73,
					}, settings, 0, 0, time.Unix(0, 0))
					if err := plan.compilationError(); err != nil {
						t.Fatalf("profile=%s span=%d variation=%d model=%s: %v",
							profile, span, variation, model, err)
					}
					for _, at := range []int64{0, plan.PeriodMillis / 3, 2 * plan.PeriodMillis / 3, plan.PeriodMillis} {
						position := plan.SampleAt(at).PositionPercent
						if position < 0 || position > 100 || math.IsNaN(position) || math.IsInf(position, 0) {
							t.Fatalf("profile=%s span=%d variation=%d model=%s sample=%d position=%g",
								profile, span, variation, model, at, position)
						}
					}
				}
			}
		}
	}
}

func TestDynamicSpanEnvelopeContentIdentityTracksTextureNotDecisionHorizon(t *testing.T) {
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 80, SpanMinPercent: 30,
		SpanProfile: DynamicSpanProfileWander, SegmentSeconds: 12,
	})
	changedHorizon := definition
	changedHorizon.SegmentSeconds = 40
	if dynamicContentID(definition) != dynamicContentID(changedHorizon) {
		t.Fatal("decision horizon changed span-envelope content identity")
	}
	changedFloor := definition
	changedFloor.SpanMinPercent = 40
	changedFloor.PhraseSeed = 0
	if dynamicContentID(definition) == dynamicContentID(changedFloor) {
		t.Fatal("span floor did not change content identity")
	}
	changedProfile := definition
	changedProfile.SpanProfile = DynamicSpanProfileContrast
	changedProfile.PhraseSeed = 0
	if dynamicContentID(definition) == dynamicContentID(changedProfile) {
		t.Fatal("span profile did not change content identity")
	}
}

func TestDynamicSpanEnvelopeRetargetPreservesHandoffContinuity(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	first := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 82, SpanMinPercent: 28,
		SpanProfile: DynamicSpanProfileWander, VariationPercent: 25,
	})
	second := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 46, SpanPercent: 72, SpanMinPercent: 24,
		SpanProfile: DynamicSpanProfileContrast, VariationPercent: 35,
	})
	previous := NewMotionPlan("wander", MotionTarget{Dynamic: &first, SpeedPercent: 73}, settings, 0, 0, time.Unix(0, 0))
	for _, phase := range []float64{0.11, 0.32, 0.57, 0.83} {
		at := int64(float64(previous.PeriodMillis) * phase)
		next := previous.Retarget("contrast", MotionTarget{Dynamic: &second, SpeedPercent: 73}, settings, at, time.Unix(1, 0))
		positionJump := math.Abs(next.SampleAt(at).PositionPercent - previous.SampleAt(at).PositionPercent)
		if positionJump > 5 {
			t.Fatalf("phase %.2f envelope retarget position jump = %.2f, want <= 5", phase, positionJump)
		}
		left, right := previous.DirectionAt(at), next.DirectionAt(at)
		if left != 0 && right != 0 && left != right {
			t.Fatalf("phase %.2f envelope retarget direction changed from %d to %d", phase, left, right)
		}
	}
}

func TestDynamicSpanEnvelopeAppearsInTraceTarget(t *testing.T) {
	settings := config.DefaultSettings().Motion
	definition := DynamicDefinition{
		CenterPercent: 50, SpanPercent: 84, SpanMinPercent: 30,
		SpanProfile: DynamicSpanProfileBreathe, VariationPercent: 22,
		SegmentSeconds: 19,
	}
	target := traceTarget(MotionTarget{
		Label: "Creative", SpeedPercent: 73, Dynamic: &definition,
	}, settings)
	if target == nil || target.MotionKind != "dynamic" ||
		target.DynamicSpanPercent != 84 || target.DynamicSpanMinPercent != 30 ||
		target.DynamicSpanProfile != DynamicSpanProfileBreathe || target.DynamicPhraseSeed == 0 ||
		target.DynamicVariationPercent != 22 || target.DynamicSegmentSeconds != 19 {
		t.Fatalf("dynamic trace target = %+v", target)
	}
}

func dynamicTwoPointCycleSpanBounds(points []CurvePoint) (float64, float64) {
	spans := dynamicTwoPointCycleSpans(points)
	minimumSpan, maximumSpan := math.MaxFloat64, 0.0
	for _, span := range spans {
		minimumSpan = math.Min(minimumSpan, span)
		maximumSpan = math.Max(maximumSpan, span)
	}
	return minimumSpan, maximumSpan
}

func dynamicTwoPointCycleSpans(points []CurvePoint) []float64 {
	spans := make([]float64, 0, len(points)/2)
	for index := 0; index+1 < len(points)-1; index += 2 {
		spans = append(spans, math.Abs(points[index+1].PositionPercent-points[index].PositionPercent))
	}
	return spans
}

func dynamicTwoPointCycleCenters(points []CurvePoint) []float64 {
	centers := make([]float64, 0, len(points)/2)
	for index := 0; index+1 < len(points)-1; index += 2 {
		centers = append(centers, (points[index+1].PositionPercent+points[index].PositionPercent)/2)
	}
	return centers
}
