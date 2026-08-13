package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func focusTestSettings() config.MotionSettings {
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	return settings
}

// A pattern that does not use the whole stroke used to shrink twice: once by
// its own authored span and again by the focus window, which is what made a
// confined pattern too subtle to feel.
func TestConfinedPatternFillsItsFocusWindow(t *testing.T) {
	definition := PatternDefinition{
		ID: "narrow", Name: "Narrow", Kind: PatternKindRoutine, CycleMillis: 6600,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 30},
			{TimeMillis: 3300, PositionPercent: 70},
			{TimeMillis: 6600, PositionPercent: 30},
		},
	}
	plan := NewMotionPlan("focused", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
		AreaFocus: &AreaFocus{MinPercent: 66, MaxPercent: 100},
	}, focusTestSettings(), 0, 0, time.Unix(0, 0))

	minimum, maximum := planPositionBounds(plan)
	if math.Abs(minimum-66) > 0.5 || math.Abs(maximum-100) > 0.5 {
		t.Fatalf("focused span = %.2f..%.2f, want the whole 66..100 window", minimum, maximum)
	}
}

func TestFocusedLoopPreservesRequestedTravelRateWithinAccelerationBudget(t *testing.T) {
	definition, _ := BuiltinPatternDefinition(PatternStroke)
	settings := focusTestSettings()
	full := NewMotionPlan("full", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 20,
	}, settings, 0, 0, time.Unix(0, 0))
	focused := NewMotionPlan("focused", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 20,
		AreaFocus: &AreaFocus{MinPercent: 0, MaxPercent: 34},
	}, settings, 0, 0, time.Unix(0, 0))

	wantPeriod := int64(math.Round(float64(full.PeriodMillis) * 0.34))
	if focused.PeriodMillis != wantPeriod {
		t.Fatalf("focused period = %dms, want %dms to preserve played travel rate", focused.PeriodMillis, wantPeriod)
	}
	fullRate := 100 / (float64(full.PeriodMillis) / 12)
	focusedRate := 34 / (float64(focused.PeriodMillis) / 12)
	if math.Abs(fullRate-focusedRate) > 0.001 {
		t.Fatalf("focused one-way rate = %.4f%%/ms, want full-range %.4f%%/ms", focusedRate, fullRate)
	}
}

func TestFocusedLoopRespectsCatalogAccelerationAndReversalBudgets(t *testing.T) {
	definition := PatternDefinition{
		ID: "slow-custom", Name: "Slow custom", Kind: PatternKindRoutine, CycleMillis: 12000,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 6000, PositionPercent: 100},
			{TimeMillis: 12000, PositionPercent: 0},
		},
	}
	focused := NewMotionPlan("focused", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
		AreaFocus: &AreaFocus{MinPercent: 0, MaxPercent: 25},
	}, focusTestSettings(), 0, 0, time.Unix(0, 0))

	wantPeriod := minimumSafeLoopPeriodAtGain(definition.Points, definition.CycleMillis, 0.25)
	if focused.PeriodMillis != wantPeriod {
		t.Fatalf("focused period = %dms, want %dms catalog-safety floor", focused.PeriodMillis, wantPeriod)
	}
	if acceleration := maximumPlanAcceleration(focused); acceleration > catalogMaxAcceleration*1.02 {
		t.Fatalf("focused acceleration = %.1f%%/s^2, over %.1f budget", acceleration, catalogMaxAcceleration)
	}
	metrics, err := MeasureCurve(definition.Points, definition.CycleMillis, true)
	if err != nil {
		t.Fatal(err)
	}
	playedGap := float64(metrics.MinReversalGapMillis) * float64(focused.PeriodMillis) / float64(definition.CycleMillis)
	if playedGap+0.01 < catalogMinReversalGap {
		t.Fatalf("focused reversal gap = %.1fms, below %dms budget", playedGap, catalogMinReversalGap)
	}
}

func TestFocusExpansionPreservesRequestedTravelRate(t *testing.T) {
	definition := PatternDefinition{
		ID: "narrow", Name: "Narrow", Kind: PatternKindRoutine, CycleMillis: 6600,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 40},
			{TimeMillis: 3300, PositionPercent: 60},
			{TimeMillis: 6600, PositionPercent: 40},
		},
	}
	full := NewMotionPlan("full", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 20,
	}, focusTestSettings(), 0, 0, time.Unix(0, 0))
	focused := NewMotionPlan("focused", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 20,
		AreaFocus: &AreaFocus{MinPercent: 0, MaxPercent: 34},
	}, focusTestSettings(), 0, 0, time.Unix(0, 0))

	gain := focused.focus.gain()
	if gain <= 1 {
		t.Fatalf("focus gain = %.2f, want expansion", gain)
	}
	fullRate := totalCurveTravel(definition.Points) * 1000 / float64(full.PeriodMillis)
	focusedRate := totalCurveTravel(definition.Points) * gain * 1000 / float64(focused.PeriodMillis)
	if math.Abs(fullRate-focusedRate) > 0.05 {
		t.Fatalf("expanded focus rate = %.2f%%/s, want %.2f", focusedRate, fullRate)
	}
}

func TestSoftAnchorPreservesRequestedTravelRate(t *testing.T) {
	definition, _ := BuiltinPatternDefinition(PatternStroke)
	full := NewMotionPlan("full", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 20,
	}, focusTestSettings(), 0, 0, time.Unix(0, 0))
	anchored := NewMotionPlan("anchored", MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 20,
		SoftAnchor: &SoftAnchor{PositionPercent: 50, WeightPercent: 50},
	}, focusTestSettings(), 0, 0, time.Unix(0, 0))

	gain := anchored.focus.gain()
	fullRate := totalCurveTravel(definition.Points) * 1000 / float64(full.PeriodMillis)
	anchoredRate := totalCurveTravel(definition.Points) * gain * 1000 / float64(anchored.PeriodMillis)
	if math.Abs(fullRate-anchoredRate) > 0.05 {
		t.Fatalf("anchored rate = %.2f%%/s, want %.2f", anchoredRate, fullRate)
	}
}

// Without a focus window the authored amplitude is the content, and nothing
// may re-expand it. A window covering everything is not a focus.
func TestUnfocusedPatternKeepsAuthoredAmplitude(t *testing.T) {
	definition := PatternDefinition{
		ID: "narrow", Name: "Narrow", Kind: PatternKindRoutine, CycleMillis: 6600,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 30},
			{TimeMillis: 3300, PositionPercent: 70},
			{TimeMillis: 6600, PositionPercent: 30},
		},
	}
	for name, focus := range map[string]*AreaFocus{
		"none":       nil,
		"whole span": {MinPercent: 0, MaxPercent: 100},
	} {
		plan := NewMotionPlan("plain", MotionTarget{
			PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
			AreaFocus: focus,
		}, focusTestSettings(), 0, 0, time.Unix(0, 0))
		minimum, maximum := planPositionBounds(plan)
		if math.Abs(minimum-30) > 0.5 || math.Abs(maximum-70) > 0.5 {
			t.Fatalf("%s: span = %.2f..%.2f, want the authored 30..70", name, minimum, maximum)
		}
	}
}

// A requested area that is too narrow to produce reliable whole-percent motion
// is widened automatically. This safety behavior does not depend on a manual
// focus setting.
func TestNarrowAreaFocusWidensToMinimumMotion(t *testing.T) {
	definition, _ := BuiltinPatternDefinition(PatternStroke)
	plan := NewMotionPlan("zoned", MotionTarget{
		PatternID: definition.ID, Pattern: &definition,
		SpeedPercent: 100, AreaFocus: &AreaFocus{MinPercent: 48, MaxPercent: 53},
	}, focusTestSettings(), 0, 0, time.Unix(0, 0))
	minimum, maximum := planPositionBounds(plan)
	if math.Abs(minimum-40) > 0.5 || math.Abs(maximum-60) > 0.5 {
		t.Fatalf("narrow focus span = %.2f..%.2f, want widened 40..60", minimum, maximum)
	}
	if maximum-minimum < minimumFocusWidthPercent-1 {
		t.Fatalf("narrow focus remained %.2f wide, want at least %d",
			maximum-minimum, minimumFocusWidthPercent)
	}
}

// The engine re-normalizes a running target on every retarget and settings
// refresh. A semantic zone must survive that without drifting inward.
func TestNormalizingATargetTwiceDoesNotDriftTheZone(t *testing.T) {
	settings := focusTestSettings()
	target := MotionTarget{
		PatternID: PatternStroke, SpeedPercent: 50,
		AreaFocus: &AreaFocus{MinPercent: 66, MaxPercent: 100},
	}
	once := NormalizeTarget(target, settings)
	twice := NormalizeTarget(once, settings)
	if *once.AreaFocus != *twice.AreaFocus {
		t.Fatalf("focus drifted on re-normalization: %+v then %+v", *once.AreaFocus, *twice.AreaFocus)
	}
	if *once.AreaFocus != (AreaFocus{MinPercent: 66, MaxPercent: 100}) {
		t.Fatalf("zone was rewritten during normalization: %+v", *once.AreaFocus)
	}
}

// A video must follow its authored positions. Contracting them toward a focus
// window is the defect docs/motion-pathway-review recorded on 2026-07-22.
func TestAreaFocusNeverTouchesClockLockedMedia(t *testing.T) {
	settings := focusTestSettings()
	media := MediaTimelineDefinition{
		ID: "clip", Name: "Clip", DurationMillis: 2000,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 5},
			{TimeMillis: 1000, PositionPercent: 95},
			{TimeMillis: 2000, PositionPercent: 5},
		},
	}
	plan := NewMotionPlan("media", MotionTarget{
		Media: &media, AreaFocus: &AreaFocus{MinPercent: 66, MaxPercent: 100},
	}, settings, 0, 0, time.Unix(0, 0))
	if plan.Target.AreaFocus != nil {
		t.Fatalf("media target carried a focus window: %+v", *plan.Target.AreaFocus)
	}
	if got := plan.SampleAt(1000).PositionPercent; math.Abs(got-95) > 0.01 {
		t.Fatalf("media position at 1000ms = %.2f, want the authored 95", got)
	}
}

// The ramp is an acceleration budget. A slow or narrowed stroke needs far less
// of it than a fast full-range one, and spending the same absolute time on it
// is what made slow patterns linger at every reversal.
func TestReversalRampShortensWithSpeedAndFocus(t *testing.T) {
	full := neutralPlaybackScale().reversalBlendMillis(100, 550)
	if full < 60 || full > maximumPatternReversalBlendMillis {
		t.Fatalf("full-speed full-range ramp = %dms, want the previous fixed ramp's order", full)
	}
	slow := playbackScale{timeFactor: 1.8, amplitudeFactor: 1}.reversalBlendMillis(100, 550)
	if slow >= full {
		t.Fatalf("slow ramp = %dms, want shorter than the full-speed %dms", slow, full)
	}
	narrow := playbackScale{timeFactor: 1.8, amplitudeFactor: 0.34}.reversalBlendMillis(100, 550)
	if narrow >= slow {
		t.Fatalf("narrowed ramp = %dms, want shorter than the unfocused %dms", narrow, slow)
	}
	if narrow < minimumPatternReversalBlendMillis {
		t.Fatalf("ramp collapsed to %dms; without a guide the zero slope eases the whole leg", narrow)
	}
}

// The reversal ramp exists to keep acceleration bounded. Shortening it must
// not push a fitted pattern past the stored-catalog budget it is derived from.
// The two promoted curves bypass authoring fit; runtime playback safety is
// covered separately for every built-in at the requested speed.
func TestReversalRampStaysInsideItsAccelerationBudget(t *testing.T) {
	for _, definition := range BuiltinPatternDefinitions() {
		if UsesExactImportedCurve(definition) {
			continue
		}
		metrics, err := MeasureCurve(definition.Points, definition.CycleMillis, true)
		if err != nil {
			t.Fatalf("%s: %v", definition.Name, err)
		}
		if metrics.MaxAccelerationPercentPerSecond2 > catalogMaxAcceleration {
			t.Fatalf("%s peaks at %.0f%%/s^2, over the %.0f budget",
				definition.Name, metrics.MaxAccelerationPercentPerSecond2, catalogMaxAcceleration)
		}
	}
}

func planPositionBounds(plan MotionPlan) (float64, float64) {
	minimum := math.MaxFloat64
	maximum := -math.MaxFloat64
	for at := int64(0); at <= plan.PeriodMillis; at += 5 {
		position := plan.SampleAt(at).PositionPercent
		minimum = math.Min(minimum, position)
		maximum = math.Max(maximum, position)
	}
	return minimum, maximum
}

func maximumPlanAcceleration(plan MotionPlan) float64 {
	const sampleMillis = int64(5)
	previous := plan.SampleAt(0).PositionPercent
	previousVelocity := 0.0
	maximum := 0.0
	for at := sampleMillis; at <= plan.PeriodMillis; at += sampleMillis {
		position := plan.SampleAt(at).PositionPercent
		velocity := (position - previous) * 1000 / float64(sampleMillis)
		if at > sampleMillis {
			acceleration := math.Abs(velocity-previousVelocity) * 1000 / float64(sampleMillis)
			maximum = math.Max(maximum, acceleration)
		}
		previous = position
		previousVelocity = velocity
	}
	return maximum
}
