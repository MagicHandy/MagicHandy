package motion

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestMediaRoundingJoinsPositionVelocityAndAcceleration(t *testing.T) {
	random := rand.New(rand.NewPCG(21, 65)) // #nosec G404 -- deterministic test data.
	for range 1000 {
		leftMillis, rightMillis := int64(10+random.IntN(2000)), int64(10+random.IntN(2000))
		points := []CurvePoint{{PositionPercent: float64(random.IntN(30))}, {TimeMillis: leftMillis, PositionPercent: 70 + float64(random.IntN(31))}, {TimeMillis: leftMillis + rightMillis, PositionPercent: float64(random.IntN(30))}}
		if random.IntN(2) == 0 {
			for i := range points {
				points[i].PositionPercent = 100 - points[i].PositionPercent
			}
		}
		curve, err := newCurve(points, leftMillis+rightMillis, false, true, MaximumMediaTimelinePoints, neutralPlaybackScale())
		if err != nil {
			t.Fatal(err)
		}
		curve.roundMediaCorners(10 + random.IntN(191))
		if len(curve.mediaFillets) != 1 {
			t.Fatal("eligible reversal was not rounded")
		}
		fillet := curve.mediaFillets[0]
		leftRate := (points[1].PositionPercent - points[0].PositionPercent) / float64(leftMillis)
		rightRate := (points[2].PositionPercent - points[1].PositionPercent) / float64(rightMillis)
		for _, boundary := range []struct {
			at      int64
			u, rate float64
		}{{fillet.start, 0, leftRate}, {fillet.end, 1, rightRate}} {
			if math.Abs(fillet.segment.velocity(boundary.u)-boundary.rate) > 1e-9 || math.Abs(fillet.segment.acceleration(boundary.u)) > 1e-9 {
				t.Fatal("rounding does not join the line with C2 continuity")
			}
			if math.Abs(curve.sampleFloat(float64(boundary.at)-1e-7)-curve.sampleFloat(float64(boundary.at)+1e-7)) > 1e-5 {
				t.Fatal("position discontinuity")
			}
		}
		for step := 0; step <= 200; step++ {
			u := float64(step) / 200
			velocity := fillet.segment.velocity(u)
			if velocity > math.Max(leftRate, rightRate)+1e-8 || velocity < math.Min(leftRate, rightRate)-1e-8 {
				t.Fatal("rounding increased peak velocity")
			}
			position := fillet.segment.position(u)
			if position > max(points[0].PositionPercent, points[1].PositionPercent, points[2].PositionPercent)+1e-8 || position < min(points[0].PositionPercent, points[1].PositionPercent, points[2].PositionPercent)-1e-8 {
				t.Fatal("rounding overshot authored reach")
			}
		}
		if !slices.Equal(points, curve.points) {
			t.Fatal("authored points changed")
		}
	}
}

func TestMediaRoundingDoesNotSilentlyDisableAtSourcePointLimit(t *testing.T) {
	points := make([]CurvePoint, MaximumMediaTimelinePoints)
	for i := range points {
		points[i] = CurvePoint{TimeMillis: int64(i) * 100, PositionPercent: float64(20 + i%2*60)}
	}
	curve, err := newCurve(points, points[len(points)-1].TimeMillis, false, true, MaximumMediaTimelinePoints, neutralPlaybackScale())
	if err != nil {
		t.Fatal(err)
	}
	curve.roundMediaCorners(60)
	if curve.mediaRounding.RoundedCorners != len(points)-2 || len(curve.points) != len(points) || curve.mediaRounding.SkippedCorners != 0 {
		t.Fatalf("dense rounding unexpectedly thinned or disabled: %+v", curve.mediaRounding)
	}
}

func TestMediaRoundingReportsTrueApexAndTimingTradeoff(t *testing.T) {
	points := []CurvePoint{{PositionPercent: 0}, {TimeMillis: 500, PositionPercent: 100}, {TimeMillis: 1000, PositionPercent: 0}}
	curve, _ := newCurve(points, 1000, false, true, MaximumMediaTimelinePoints, neutralPlaybackScale())
	curve.roundMediaCorners(100)
	if math.Abs(curve.Sample(500)-92.5) > 1e-8 || math.Abs(curve.mediaRounding.PeakReductionPercent-7.5) > 1e-8 || curve.mediaRounding.PeakShiftMillis > 1e-6 {
		t.Fatalf("symmetric rounding = %+v at %v", curve.mediaRounding, curve.Sample(500))
	}
	points[2].TimeMillis = 1500
	curve, _ = newCurve(points, 1500, false, true, MaximumMediaTimelinePoints, neutralPlaybackScale())
	curve.roundMediaCorners(100)
	if curve.mediaRounding.PeakShiftMillis <= 0 {
		t.Fatal("asymmetric peak timing tradeoff was hidden")
	}
}

func TestMediaRoundingKeepsDwellAndReportsTooShortCorners(t *testing.T) {
	points := []CurvePoint{{PositionPercent: 20}, {TimeMillis: 8, PositionPercent: 80}, {TimeMillis: 16, PositionPercent: 20}, {TimeMillis: 1000, PositionPercent: 80}, {TimeMillis: 1500, PositionPercent: 80}, {TimeMillis: 2000, PositionPercent: 20}}
	curve, _ := newCurve(points, 2000, false, true, MaximumMediaTimelinePoints, neutralPlaybackScale())
	curve.roundMediaCorners(200)
	if curve.mediaRounding.SkippedCorners != 2 {
		t.Fatalf("skipped corners = %+v", curve.mediaRounding)
	}
	for at := int64(1000); at <= 1500; at++ {
		if curve.Sample(at) != 80 {
			t.Fatal("rounding changed authored dwell")
		}
	}
}

func TestMediaRoundingCompilesAfterSpeedCapAndPreservesClock(t *testing.T) {
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings := config.DefaultSettings().Motion
		settings.HandyModel, settings.ApplyVideoSpeedLimit, settings.SpeedMaxPercent = model, true, 25
		media := MediaTimelineDefinition{ID: "clock", Name: "Clock", DurationMillis: 1000, RoundingMillis: 100, Points: []CurvePoint{{PositionPercent: 0}, {TimeMillis: 500, PositionPercent: 100}, {TimeMillis: 1000, PositionPercent: 0}}}
		plan := NewMotionPlan("media", MotionTarget{Source: TargetSourceMedia, Media: &media}, settings, 0, 0, time.Time{})
		if err := plan.compilationError(); err != nil {
			t.Fatal(err)
		}
		if plan.PeriodMillis != 1000 || plan.Loop || plan.Target.MediaRoundingEffect.RoundedCorners != 1 {
			t.Fatalf("compiled media = %+v", plan.Target.Media)
		}
		if peak := plan.curve.maximumVelocityPerMillis() * 1000; peak > referenceTravelRateForSpeed(25, model)+1e-6 {
			t.Fatalf("speed cap exceeded: %v", peak)
		}
		if plan.Target.MediaRoundingEffect.PeakReductionPercent >= 7.5 {
			t.Fatal("reported pre-cap rather than compiled peak reduction")
		}
	}
}
