package motion

import (
	"math"
	"slices"
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

func TestFocusedLoopRespectsAuthoredAccelerationBudget(t *testing.T) {
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

	// A quarter-distance loop needs at least half its authored period to keep
	// acceleration at or below the source pattern (distance / time^2).
	if focused.PeriodMillis != 6000 {
		t.Fatalf("focused period = %dms, want 6000ms authored-acceleration floor", focused.PeriodMillis)
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

// The configured range is the working range. A zone request subdivides it, so
// no model request can move motion outside the region the user chose.
func TestConfiguredFocusRangeContainsEveryZoneRequest(t *testing.T) {
	settings := focusTestSettings()
	settings.FocusMinPercent = 20
	settings.FocusMaxPercent = 80
	definition, _ := BuiltinPatternDefinition(PatternStroke)

	for _, zone := range []*AreaFocus{
		nil,
		{MinPercent: 66, MaxPercent: 100},
		{MinPercent: 33, MaxPercent: 67},
		{MinPercent: 0, MaxPercent: 34},
	} {
		plan := NewMotionPlan("zoned", MotionTarget{
			PatternID: definition.ID, Pattern: &definition,
			SpeedPercent: 100, AreaFocus: zone,
		}, settings, 0, 0, time.Unix(0, 0))
		minimum, maximum := planPositionBounds(plan)
		if minimum < 19.5 || maximum > 80.5 {
			t.Fatalf("zone %+v escaped the configured 20-80 range: %.2f..%.2f", zone, minimum, maximum)
		}
		if maximum-minimum < config.MinimumFocusWidthPercent-1 {
			t.Fatalf("zone %+v collapsed to %.2f wide, want at least %d",
				zone, maximum-minimum, config.MinimumFocusWidthPercent)
		}
	}
}

// The engine re-normalizes a running target on every retarget and settings
// refresh. A zone kept in full-stroke coordinates survives that; a zone
// rewritten into the configured range would drift inward each time.
func TestNormalizingATargetTwiceDoesNotDriftTheZone(t *testing.T) {
	settings := focusTestSettings()
	settings.FocusMinPercent = 20
	settings.FocusMaxPercent = 80
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
func TestConfiguredFocusRangeNeverTouchesClockLockedMedia(t *testing.T) {
	settings := focusTestSettings()
	settings.FocusMinPercent = 40
	settings.FocusMaxPercent = 60
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
// not push a generated pattern past the budget it is derived from. Promoted
// curated curves are deliberately not time-fitted (content_curated.go), so
// their authored peaks are out of scope here.
func TestReversalRampStaysInsideItsAccelerationBudget(t *testing.T) {
	for _, definition := range BuiltinPatternDefinitions() {
		if slices.Contains(definition.Tags, TagCurated) {
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
