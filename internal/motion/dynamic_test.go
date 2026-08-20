package motion

import (
	"math"
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
