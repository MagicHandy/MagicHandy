package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestCreativeSpeedSelectionProducesDistinctCommandedRates(t *testing.T) {
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent: 50, SpanPercent: 38, SpanMinPercent: 20,
		SpanProfile: DynamicSpanProfileWander, VariationPercent: 68,
	})
	content := dynamicContent(definition)
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	settings.HandyModel = config.HandyModelOriginal

	var previous float64
	var first float64
	for _, speed := range []int{42, 52, 62, 72} {
		plan := NewMotionPlan("creative-speed-fidelity", MotionTarget{
			Dynamic: &definition, SpeedPercent: speed,
		}, settings, 0, 0, time.Unix(0, 0))
		if err := plan.compilationError(); err != nil {
			t.Fatalf("speed %d: %v", speed, err)
		}
		rate := totalCurveTravel(content.points) * 1000 / float64(plan.PeriodMillis)
		peakVelocity := approximateMaximumPlayedVelocity(plan)
		acceleration := maximumPlanAcceleration(plan)
		jerk := maximumPlanJerk(plan)
		t.Logf("speed=%d mean=%.1f%%/s peak=%.1f%%/s requested=%.1f%%/s accel=%.0f%%/s2 jerk=%.0f%%/s3 period=%dms",
			speed, rate, peakVelocity, referenceTravelRateForSpeed(speed, settings.HandyModel),
			acceleration, jerk, plan.PeriodMillis)
		if first == 0 {
			first = rate
		}
		requested := referenceTravelRateForSpeed(speed, settings.HandyModel)
		if rate > requested*1.015 {
			t.Errorf("speed %d mean %.1f%%/s exceeds selected effective pace %.1f%%/s",
				speed, rate, requested)
		}
		if rate < requested*0.985 {
			if !plan.Perceptual.Pace.Limited || len(plan.Perceptual.Pace.Limiters) == 0 {
				t.Errorf("speed %d mean %.1f%%/s is below request %.1f without saturation diagnostics: %+v",
					speed, rate, requested, plan.Perceptual.Pace)
			}
		} else if plan.Perceptual.Pace.Limited {
			t.Errorf("speed %d reached request but is marked limited: %+v", speed, plan.Perceptual.Pace)
		}
		devicePeak := referenceTravelRateForSpeed(100, settings.HandyModel)
		if peakVelocity > devicePeak*1.005 {
			t.Errorf("speed %d peak %.1f%%/s exceeds calibrated device peak %.1f%%/s",
				speed, peakVelocity, devicePeak)
		}
		if previous > 0 && rate < previous*0.995 {
			t.Errorf("speed %d commanded rate %.1f%%/s regressed after %.1f%%/s",
				speed, rate, previous)
		}
		if acceleration > runtimeMaxAccelerationPercentPerSecond2*1.002 {
			t.Errorf("speed %d acceleration %.1f exceeds %.1f",
				speed, acceleration, runtimeMaxAccelerationPercentPerSecond2)
		}
		previous = rate
	}
	if previous < first*1.20 {
		t.Fatalf("42-72%% Creative rate spread is only %.1f%% (%.1f to %.1f%%/s); want at least 20%% before saturation",
			(previous/first-1)*100, first, previous)
	}
}

func TestCreativeVelocityFitAcrossHandyModelsAndPhrases(t *testing.T) {
	phrases := map[string]DynamicDefinition{
		"narrow-steady": {
			CenterPercent: 50, SpanPercent: 20, SpanMinPercent: 20,
			SpanProfile: DynamicSpanProfileSteady,
		},
		"broad-contrast": {
			CenterPercent: 50, SpanPercent: 86, SpanMinPercent: 22,
			SpanProfile: DynamicSpanProfileContrast, VariationPercent: 78,
		},
		"anchor-wander": {
			Anchors: []DynamicAnchor{
				{Name: "base", PositionPercent: 8},
				{Name: "middle", PositionPercent: 50},
				{Name: "tip", PositionPercent: 92},
			},
			SpanMinPercent: 24, SpanProfile: DynamicSpanProfileWander,
			VariationPercent: 72,
		},
		"sections": {
			Sections: []DynamicSection{
				{
					CenterPercent: 42, SpanPercent: 68, SpanMinPercent: 22,
					SpanProfile: DynamicSpanProfileWander, VariationPercent: 62, Cycles: 3,
				},
				{
					CenterPercent: 64, SpanPercent: 58, SpanMinPercent: 24,
					SpanProfile: DynamicSpanProfileContrast, VariationPercent: 76, Cycles: 4,
				},
			},
		},
	}
	models := []string{
		config.HandyModelOriginal,
		config.HandyModel2Standard,
		config.HandyModel2Pro,
	}
	for name, source := range phrases {
		definition := NormalizeDynamicDefinition(source)
		for _, model := range models {
			settings := config.DefaultSettings().Motion
			settings.SpeedMinPercent = 1
			settings.SpeedMaxPercent = 100
			settings.HandyModel = model
			previousRate := 0.0
			firstRate := 0.0
			for _, speed := range []int{20, 40, 60, 80, 100} {
				plan := NewMotionPlan("creative-profile-matrix", MotionTarget{
					Dynamic: &definition, SpeedPercent: speed,
				}, settings, 0, 0, time.Unix(0, 0))
				if err := plan.compilationError(); err != nil {
					t.Fatalf("phrase=%s model=%s speed=%d: %v", name, model, speed, err)
				}
				rate := totalCurveTravel(plan.curve.authoredKnots) * 1000 / float64(plan.PeriodMillis)
				peak := approximateMaximumPlayedVelocity(plan)
				requested := referenceTravelRateForSpeed(speed, model)
				t.Logf("phrase=%s model=%s speed=%d mean=%.1f peak=%.1f requested=%.1f",
					name, model, speed, rate, peak, requested)
				devicePeak := referenceTravelRateForSpeed(100, model)
				if peak > devicePeak*1.01 {
					t.Errorf("phrase=%s model=%s speed=%d peak %.1f exceeds device peak %.1f",
						name, model, speed, peak, devicePeak)
				}
				limited := rate < requested*0.985
				if limited && (!plan.Perceptual.Pace.Limited || len(plan.Perceptual.Pace.Limiters) == 0) {
					t.Errorf("phrase=%s model=%s speed=%d effective mean %.1f is below request %.1f without diagnostics: %+v",
						name, model, speed, rate, requested, plan.Perceptual.Pace)
				}
				if firstRate == 0 {
					firstRate = rate
				}
				if previousRate > 0 && rate < previousRate*0.995 {
					t.Errorf("phrase=%s model=%s speed=%d mean %.1f regressed after %.1f",
						name, model, speed, rate, previousRate)
				}
				if name != "narrow-steady" && speed <= 80 && previousRate > 0 && !limited && rate < previousRate*1.035 {
					t.Errorf("phrase=%s model=%s speed=%d mean %.1f after %.1f; want visible separation through the normal range",
						name, model, speed, rate, previousRate)
				}
				if acceleration := maximumPlanAcceleration(plan); acceleration > runtimeMaxAccelerationPercentPerSecond2*1.002 {
					t.Errorf("phrase=%s model=%s speed=%d acceleration %.1f exceeds %.1f",
						name, model, speed, acceleration, runtimeMaxAccelerationPercentPerSecond2)
				}
				if jerk := maximumPlanJerk(plan); jerk > runtimeMaxJerkPercentPerSecond3*1.002 {
					t.Errorf("phrase=%s model=%s speed=%d jerk %.1f exceeds %.1f",
						name, model, speed, jerk, runtimeMaxJerkPercentPerSecond3)
				}
				previousRate = rate
			}
			if previousRate < firstRate*1.25 {
				t.Errorf("phrase=%s model=%s full speed range changes mean rate only %.1f%%",
					name, model, (previousRate/firstRate-1)*100)
			}
		}
	}
}

func TestCreativeResolvedTimingDoesNotInheritPatternHalfSecondFloor(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	settings.HandyModel = config.HandyModelOriginal
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent:  50,
		SpanPercent:    20,
		SpanMinPercent: 20,
		SpanProfile:    DynamicSpanProfileSteady,
	})
	plan := NewMotionPlan("creative-real-floor", MotionTarget{
		Dynamic: &definition, SpeedPercent: 100,
	}, settings, 0, 0, time.Unix(0, 0))
	if err := plan.compilationError(); err != nil {
		t.Fatal(err)
	}
	if plan.PeriodMillis >= minimumBurstCycleMillis {
		t.Fatalf("Creative period = %dms, still pinned to generic %dms pattern floor",
			plan.PeriodMillis, minimumBurstCycleMillis)
	}
	if plan.Perceptual.Pace.RequestedPercent != 100 ||
		plan.Perceptual.Pace.EffectivePercent <= 0 ||
		!plan.Perceptual.Pace.Limited || len(plan.Perceptual.Pace.Limiters) == 0 {
		t.Fatalf("Creative pace diagnostics do not explain the physical limit: %+v", plan.Perceptual.Pace)
	}
}

func TestCreativeLongStrokeHasCruiseLikeVelocityBody(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	settings.HandyModel = config.HandyModelOriginal
	definition := NormalizeDynamicDefinition(DynamicDefinition{
		CenterPercent:  50,
		SpanPercent:    86,
		SpanMinPercent: 86,
		SpanProfile:    DynamicSpanProfileSteady,
	})
	plan := NewMotionPlan("creative-cruise-body", MotionTarget{
		Dynamic: &definition, SpeedPercent: 73,
	}, settings, 0, 0, time.Unix(0, 0))
	if err := plan.compilationError(); err != nil {
		t.Fatal(err)
	}
	ratio := plan.Perceptual.CommandedPeakVelocityPerSecond /
		plan.Perceptual.CommandedMeanTravelPerSecond
	if ratio > 1.38 {
		t.Fatalf("Creative velocity peak/mean ratio = %.3f, want a sustained stroke body rather than a rounded linear surge", ratio)
	}
	if acceleration := maximumPlanAcceleration(plan); acceleration > runtimeMaxAccelerationPercentPerSecond2*1.002 {
		t.Fatalf("acceleration %.1f exceeds %.1f", acceleration, runtimeMaxAccelerationPercentPerSecond2)
	}
	if jerk := maximumPlanJerk(plan); jerk > runtimeMaxJerkPercentPerSecond3*1.002 {
		t.Fatalf("jerk %.1f exceeds %.1f", jerk, runtimeMaxJerkPercentPerSecond3)
	}
}

func TestCreativeVariableSpanIsLocallyDiverseAfterCompilation(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 1
	settings.SpeedMaxPercent = 100
	settings.HandyModel = config.HandyModelOriginal
	for _, profile := range []string{
		DynamicSpanProfileBreathe,
		DynamicSpanProfileWander,
		DynamicSpanProfileContrast,
	} {
		definition := NormalizeDynamicDefinition(DynamicDefinition{
			CenterPercent: 50, SpanPercent: 42, SpanMinPercent: 20,
			SpanProfile: profile, VariationPercent: 68,
		})
		plan := NewMotionPlan("creative-local-diversity", MotionTarget{
			Dynamic: &definition, SpeedPercent: 58,
		}, settings, 0, 0, time.Unix(0, 0))
		if err := plan.compilationError(); err != nil {
			t.Fatalf("profile=%s: %v", profile, err)
		}
		minimumCV, minimumRange, minimumDistinct := minimumCompiledStrokeDiversity(plan, 12*time.Second)
		t.Logf("profile=%s minimum_12s_cv=%.3f range=%.1f distinct=%d",
			profile, minimumCV, minimumRange, minimumDistinct)
		wantedCV := 0.08
		if profile == DynamicSpanProfileBreathe {
			wantedCV = 0.05
		}
		if minimumCV < wantedCV {
			t.Errorf("profile=%s has a locally regular 12-second window with stroke CV %.3f; want >= %.2f",
				profile, minimumCV, wantedCV)
		}
		if minimumRange < 6 {
			t.Errorf("profile=%s changes stroke length only %.1f%% in one 12-second window; want >= 6%%",
				profile, minimumRange)
		}
		if minimumDistinct < 5 {
			t.Errorf("profile=%s has only %d distinct whole-percent stroke lengths in one 12-second window; want >= 5",
				profile, minimumDistinct)
		}
	}
}

func minimumCompiledStrokeDiversity(plan MotionPlan, window time.Duration) (float64, float64, int) {
	type stroke struct {
		at     int64
		length float64
	}
	period := plan.PeriodMillis
	strokes := make([]stroke, 0, len(plan.curve.authoredKnots)*2)
	for pass := int64(0); pass < 2; pass++ {
		for index := 1; index < len(plan.curve.authoredKnots); index++ {
			left, right := plan.curve.authoredKnots[index-1], plan.curve.authoredKnots[index]
			strokes = append(strokes, stroke{
				at:     pass*period + (left.TimeMillis+right.TimeMillis)/2,
				length: math.Abs(right.PositionPercent - left.PositionPercent),
			})
		}
	}
	windowMillis := window.Milliseconds()
	minimumCV, minimumRange, minimumDistinct := math.MaxFloat64, math.MaxFloat64, int(^uint(0)>>1)
	for start := int64(0); start < period; start += 1000 {
		lengths := make([]float64, 0, 64)
		for _, candidate := range strokes {
			if candidate.at >= start && candidate.at < start+windowMillis {
				lengths = append(lengths, candidate.length)
			}
		}
		if len(lengths) < 6 {
			continue
		}
		mean := 0.0
		minimum, maximum := math.MaxFloat64, 0.0
		distinct := map[int]bool{}
		for _, length := range lengths {
			mean += length
			minimum = math.Min(minimum, length)
			maximum = math.Max(maximum, length)
			distinct[int(math.Round(length))] = true
		}
		mean /= float64(len(lengths))
		variance := 0.0
		for _, length := range lengths {
			variance += (length - mean) * (length - mean)
		}
		cv := math.Sqrt(variance/float64(len(lengths))) / mean
		minimumCV = math.Min(minimumCV, cv)
		minimumRange = math.Min(minimumRange, maximum-minimum)
		minimumDistinct = min(minimumDistinct, len(distinct))
	}
	return minimumCV, minimumRange, minimumDistinct
}

func maximumPlanJerk(plan MotionPlan) float64 {
	timeFactor := float64(plan.PeriodMillis) / float64(plan.curve.duration)
	return plan.curve.maximumJerkPerMillis3() * plan.focus.gain() /
		(timeFactor * timeFactor * timeFactor) * 1e9
}

func approximateMaximumPlayedVelocity(plan MotionPlan) float64 {
	maximum := 0.0
	step := max(int64(1), plan.PeriodMillis/20000)
	for at := int64(0); at < plan.PeriodMillis; at += step {
		maximum = math.Max(maximum, math.Abs(dynamicPlayedVelocity(plan, at)))
	}
	return maximum
}
