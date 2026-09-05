package motion

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func labTestRequest() LabRequest {
	return LabRequest{SpeedPercent: 30, CenterPercent: 50, SpanPercent: 90,
		SpanMinPercent: 25, SpanProfile: DynamicSpanProfileWander, VariationPercent: 0,
		RangeAnchorPercent: 0, OutboundTimePercent: 35, Seed: 17}
}

func TestMotionLabNeutralIsExactCreativeBaseline(t *testing.T) {
	request := labTestRequest()
	request.RangeAnchorPercent, request.OutboundTimePercent = 50, 50
	preview, err := PreviewMotionLab(request, config.DefaultSettings().Motion)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range preview.Candidates {
		if candidate.Target.Dynamic.Experiment != nil ||
			!reflect.DeepEqual(candidate.Samples, preview.Candidates[0].Samples) {
			t.Fatalf("neutral %s changed Creative output", candidate.Method)
		}
	}
}

func TestMotionLabAnchoringAndTimingSurviveCompilation(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	request := labTestRequest()
	for _, anchor := range []int{0, 100} {
		request.RangeAnchorPercent = anchor
		target, err := request.Target("anchored")
		if err != nil {
			t.Fatal(err)
		}
		plan := NewMotionPlan("test", target, settings, 0, 0, time.Unix(0, 0))
		for index, point := range plan.curve.authoredKnots[:len(plan.curve.authoredKnots)-1] {
			outgoing := plan.curve.authoredKnots[index+1].PositionPercent - point.PositionPercent
			if anchor == 0 && outgoing > 0 && math.Abs(point.PositionPercent-5) > 1e-8 {
				t.Fatalf("base anchor drifted to %.6f", point.PositionPercent)
			}
			if anchor == 100 && outgoing < 0 && math.Abs(point.PositionPercent-95) > 1e-8 {
				t.Fatalf("tip anchor drifted to %.6f", point.PositionPercent)
			}
		}
	}
	request = labTestRequest()
	request.SpanProfile, request.SpanMinPercent = DynamicSpanProfileSteady, 90
	preview, err := PreviewMotionLab(request, settings)
	if err != nil {
		t.Fatal(err)
	}
	baseline, directional := preview.Candidates[0], preview.Candidates[2]
	if math.Abs(baseline.OutboundTimePercent-50) > 0.1 || directional.OutboundTimePercent > 42 {
		t.Fatalf("directional timing collapsed: baseline %.2f, candidate %.2f", baseline.OutboundTimePercent, directional.OutboundTimePercent)
	}
	if baseline.Perceptual.PositionMinPercent != directional.Perceptual.PositionMinPercent ||
		baseline.Perceptual.PositionMaxPercent != directional.Perceptual.PositionMaxPercent {
		t.Fatal("timing changed geometry")
	}
}

func TestMotionLabUsesSharedEnvelopeAcrossSpeeds(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, speed := range []int{10, 35, 75, 100} {
		request := labTestRequest()
		request.SpeedPercent, request.VariationPercent = speed, 65
		preview, err := PreviewMotionLab(request, settings)
		if err != nil {
			t.Fatal(err)
		}
		for _, candidate := range preview.Candidates {
			if candidate.MaximumAcceleration > runtimeMaxAccelerationPercentPerSecond2*1.002 ||
				candidate.MaximumJerk > runtimeMaxJerkPercentPerSecond3*1.002 ||
				candidate.Perceptual.CommandedPeakVelocityPerSecond > referenceTravelRateForSpeed(100, settings.HandyModel)*1.002 {
				t.Fatalf("%d/%s violated shared envelope: %+v", speed, candidate.Method, candidate.Perceptual)
			}
			for index, sample := range candidate.Samples {
				if sample.PositionPercent < 0 || sample.PositionPercent > 100 ||
					index > 0 && sample.TimeMillis <= candidate.Samples[index-1].TimeMillis {
					t.Fatalf("invalid preview sample %d: %+v", index, sample)
				}
			}
			t.Logf("speed=%d method=%s effective=%.1f outbound=%.1f%% localCV=%.3f limits=%v", speed, candidate.Method,
				candidate.Perceptual.Pace.EffectivePercent, candidate.OutboundTimePercent,
				candidate.Perceptual.MinimumLocalStrokeCV, candidate.Perceptual.Pace.Limiters)
		}
	}
}

func TestMotionLabInvalidInputsAndIdentity(t *testing.T) {
	request := labTestRequest()
	if _, err := request.Target("raw_points"); err == nil {
		t.Fatal("accepted unknown method")
	}
	request.OutboundTimePercent = 0
	if _, err := request.Target("creative"); err == nil {
		t.Fatal("accepted invalid timing")
	}
	request = labTestRequest()
	base, _ := request.Target("creative")
	anchored, _ := request.Target("anchored")
	if dynamicContentID(*base.Dynamic) == dynamicContentID(*anchored.Dynamic) {
		t.Fatal("experimental geometry was omitted from retarget identity")
	}
}
