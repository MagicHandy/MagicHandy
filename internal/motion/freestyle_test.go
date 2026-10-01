package motion

import (
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func freestyleSettings() config.MotionSettings {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 20, 80
	return settings
}

func freestyleControls() FreestyleControls {
	return FreestyleControls{PacePercent: 50, LengthPercent: 65, FocusPercent: 50, RoamingPercent: 50,
		VarietyPercent: 60, EnergyPercent: 100}
}

func freestyleWindow(start, strokes int, keyframes ...FreestyleKeyframe) FlowSpec {
	if len(keyframes) == 0 {
		keyframes = []FreestyleKeyframe{{Controls: freestyleControls()}}
	}
	return NewFreestyleFlow(0x5eed, 80, FreestyleSpec{StartStroke: start, Strokes: strokes,
		MinSpeedPercent: 20, Keyframes: keyframes})
}

func compileFreestyleWindow(t *testing.T, spec FlowSpec) (MotionTarget, *preparedMotion) {
	t.Helper()
	target, err := FlowTarget(spec, freestyleSettings())
	if err != nil {
		t.Fatal(err)
	}
	return target, target.prepared
}

// strokePoints returns the curve points of one stroke, relative to its start.
func strokePoints(content *preparedMotion, stroke int) []CurvePoint {
	window := content.stream
	index := stroke - window.startStroke
	start, end := window.strokeStarts[index], window.strokeStarts[index+1]
	var points []CurvePoint
	for _, point := range content.curve.points {
		if point.TimeMillis >= start && point.TimeMillis <= end {
			points = append(points, CurvePoint{point.TimeMillis - start, point.PositionPercent})
		}
	}
	return points
}

func sameStrokes(t *testing.T, a, b *preparedMotion, from, to int) {
	t.Helper()
	for stroke := from; stroke < to; stroke++ {
		left, right := strokePoints(a, stroke), strokePoints(b, stroke)
		if len(left) != len(right) {
			t.Fatalf("stroke %d has %d points in one window and %d in another", stroke, len(left), len(right))
		}
		for index := range left {
			if left[index] != right[index] {
				t.Fatalf("stroke %d differs at point %d: %v != %v", stroke, index, left[index], right[index])
			}
		}
	}
}

func TestFreestyleWindowsShareTheirStrokesExactly(t *testing.T) {
	_, first := compileFreestyleWindow(t, freestyleWindow(0, 120))
	_, second := compileFreestyleWindow(t, freestyleWindow(37, 120))
	_, third := compileFreestyleWindow(t, freestyleWindow(101, 60))
	sameStrokes(t, first, second, 37, 120)
	sameStrokes(t, second, third, 101, 157)
}

func TestFreestylePlanKeepsTheFittedClock(t *testing.T) {
	settings := freestyleSettings()
	for _, controls := range []FreestyleControls{
		freestyleControls(),
		{PacePercent: 100, LengthPercent: 0, FocusPercent: 100, RoamingPercent: 0, VarietyPercent: 100, AccentPercent: 100, EnergyPercent: 100},
		{PacePercent: 0, LengthPercent: 100, FocusPercent: 0, RoamingPercent: 100, VarietyPercent: 0, AccentPercent: -100, EnergyPercent: 0, TeasePercent: 100},
	} {
		spec := freestyleWindow(0, 160, FreestyleKeyframe{Controls: controls})
		target, err := FlowTarget(spec, settings)
		if err != nil {
			t.Fatalf("%+v: %v", controls, err)
		}
		plan := NewMotionPlan("freestyle", target, settings, 0, 0, time.Unix(0, 0))
		if plan.compilationError() != nil || plan.Loop {
			t.Fatalf("%+v: %v loop=%v", controls, plan.compilationError(), plan.Loop)
		}
		// Any plan-level retiming would desynchronize the stroke index that a
		// continuation relies on, so the fitted window must already be safe.
		if plan.PeriodMillis != plan.curve.duration {
			t.Fatalf("%+v: plan retimed the window: period %d, curve %d", controls, plan.PeriodMillis, plan.curve.duration)
		}
		for _, point := range plan.curve.points {
			if point.PositionPercent < 0 || point.PositionPercent > 100 {
				t.Fatalf("%+v: position %v outside the band", controls, point.PositionPercent)
			}
		}
	}
}

func continueFreestyle(t *testing.T, previous MotionPlan, at int64, spec FlowSpec) MotionPlan {
	t.Helper()
	next := previous.retargetFromState("next", MotionTarget{Label: "Freestyle", Flow: &spec}, freestyleSettings(), at,
		previous.SampleAt(at).PositionPercent, previous.DirectionAt(at), previous.VelocityAt(at), time.Unix(0, 0))
	if next.compilationError() != nil {
		t.Fatal(next.compilationError())
	}
	return next
}

func TestFreestyleContinuationNeedsNoBlend(t *testing.T) {
	firstTarget, _ := compileFreestyleWindow(t, freestyleWindow(0, 120))
	previous := NewMotionPlan("first", firstTarget, freestyleSettings(), 0, 0, time.Unix(0, 0))
	at := previous.PeriodMillis * 3 / 5
	playing := previous.freestyleProgress(at, at).PlayingStroke
	next := continueFreestyle(t, previous, at, freestyleWindow(playing, 120))
	if transitionRequired(previous, nil, next, at) {
		t.Fatal("a continuation of the same stream needed a crossfade")
	}
	for offset := int64(0); offset < 20_000; offset += 37 {
		if math.Abs(previous.SampleAt(at+offset).PositionPercent-next.SampleAt(at+offset).PositionPercent) > 1e-6 {
			t.Fatalf("continuation diverged %d ms after the handoff", offset)
		}
	}
}

func TestFreestyleRampStartsAfterQueuedMotion(t *testing.T) {
	firstTarget, _ := compileFreestyleWindow(t, freestyleWindow(0, 120))
	previous := NewMotionPlan("first", firstTarget, freestyleSettings(), 0, 0, time.Unix(0, 0))
	at := previous.PeriodMillis / 3
	progress := previous.freestyleProgress(at-1200, at)
	changed := freestyleControls()
	changed.PacePercent, changed.LengthPercent, changed.FocusPercent = 95, 20, 90
	keyframes := []FreestyleKeyframe{{Controls: freestyleControls()}, {Stroke: progress.EditableStroke, RampStrokes: 8, Controls: changed}}
	next := continueFreestyle(t, previous, at, freestyleWindow(progress.PlayingStroke, 120, keyframes...))
	if transitionRequired(previous, nil, next, at) {
		t.Fatal("a control ramp after the editable stroke still touched queued motion")
	}
	diverged := false
	for offset := int64(0); offset < 30_000 && !diverged; offset += 25 {
		diverged = math.Abs(previous.SampleAt(at+offset).PositionPercent-next.SampleAt(at+offset).PositionPercent) > 1
	}
	if !diverged {
		t.Fatal("the new controls never changed the stream")
	}
}

func TestCompactedKeyframesReproduceTheStream(t *testing.T) {
	controls := freestyleControls()
	quick, slow := controls, controls
	quick.PacePercent, quick.LengthPercent = 90, 30
	slow.PacePercent, slow.FocusPercent = 15, 80
	keyframes := []FreestyleKeyframe{{Controls: controls}, {Stroke: 20, RampStrokes: 10, Controls: quick},
		{Stroke: 45, RampStrokes: 30, Controls: slow}, {Stroke: 60, RampStrokes: 12, Controls: quick}}
	compacted := CompactFreestyleKeyframes(keyframes, 85)
	// The first ramp settled before the second began, so it folds into the
	// starting state; the second was still ramping when the third began.
	if len(compacted) != 3 || compacted[0].Controls != quick {
		t.Fatalf("compaction kept %d keyframes: %+v", len(compacted), compacted)
	}
	// The window and its context only depend on the history that remains; the
	// compacted keyframes must reproduce them bit for bit.
	_, full := compileFreestyleWindow(t, freestyleWindow(85, 60, keyframes...))
	_, short := compileFreestyleWindow(t, freestyleWindow(85, 60, compacted...))
	sameStrokes(t, full, short, 85, 145)
}
