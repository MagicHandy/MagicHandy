package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestMotionTargetClamping(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 20
	settings.SpeedMaxPercent = 60

	target := NormalizeTarget(MotionTarget{
		SpeedPercent: 99,
		AreaFocus: &AreaFocus{
			MinPercent: 90,
			MaxPercent: 10,
		},
		SoftAnchor: &SoftAnchor{
			PositionPercent: 150,
			WeightPercent:   120,
		},
	}, settings)

	if target.PatternID != PatternStroke {
		t.Fatalf("pattern = %q, want default stroke", target.PatternID)
	}
	if target.SpeedPercent != 60 {
		t.Fatalf("speed = %d, want clamped max 60", target.SpeedPercent)
	}
	if target.AreaFocus.MinPercent >= target.AreaFocus.MaxPercent {
		t.Fatalf("area focus = %+v, want usable range", target.AreaFocus)
	}
	if target.SoftAnchor.PositionPercent != 100 || target.SoftAnchor.WeightPercent != 100 {
		t.Fatalf("soft anchor = %+v, want clamped anchor", target.SoftAnchor)
	}
}

func TestSamplerSupportsFixedPatternsAreaFocusAndSoftAnchor(t *testing.T) {
	settings := config.DefaultSettings().Motion
	plan := NewMotionPlan("pulse", MotionTarget{
		PatternID:    PatternPulse,
		SpeedPercent: 50,
		AreaFocus: &AreaFocus{
			MinPercent: 20,
			MaxPercent: 70,
		},
	}, settings, 0, 0, time.Unix(0, 0))

	low := plan.SampleAt(0)
	peak := plan.SampleAt(plan.PeriodMillis / 5)
	if low.PositionPercent < 20 || peak.PositionPercent > 70 {
		t.Fatalf("samples escaped focus: low=%+v peak=%+v", low, peak)
	}
	if peak.PositionPercent <= low.PositionPercent {
		t.Fatalf("fixed pulse peak = %d, want above low %d", peak.PositionPercent, low.PositionPercent)
	}

	anchored := NewMotionPlan("anchored", MotionTarget{
		PatternID:    PatternStroke,
		SpeedPercent: 50,
		SoftAnchor:   &SoftAnchor{PositionPercent: 50, WeightPercent: 50},
	}, settings, 0, 0, time.Unix(0, 0))
	if got := anchored.SampleAt(0).PositionPercent; got != 25 {
		t.Fatalf("anchored sample = %d, want 25", got)
	}
}

func TestSamePatternRetargetPreservesPhase(t *testing.T) {
	settings := config.DefaultSettings().Motion
	started := time.Unix(0, 0)
	plan := NewMotionPlan("initial", MotionTarget{
		PatternID:    PatternStroke,
		SpeedPercent: 80,
	}, settings, 0.125, 0, started)
	streamMillis := plan.PeriodMillis / 3
	phase := plan.PhaseAt(streamMillis)

	next := plan.Retarget("next", MotionTarget{
		PatternID:    PatternStroke,
		SpeedPercent: 30,
	}, settings, streamMillis, started.Add(time.Second))

	if !next.PhasePreserved {
		t.Fatal("same-pattern retarget did not mark phase preservation")
	}
	if !almostEqual(next.PhaseAt(streamMillis), phase) {
		t.Fatalf("phase = %.6f, want preserved %.6f", next.PhaseAt(streamMillis), phase)
	}
	if next.PeriodMillis == plan.PeriodMillis {
		t.Fatal("retarget did not apply new speed period")
	}
}

func almostEqual(left float64, right float64) bool {
	return math.Abs(left-right) < 0.000001
}
