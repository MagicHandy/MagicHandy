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
	curve := content.buildCurve(neutralPlaybackScale())
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
