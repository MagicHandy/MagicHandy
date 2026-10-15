package vibration

import (
	"math"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/companion"
)

// strokes builds a full-span stroke timeline with the given half-period.
func strokes(t *testing.T, halfPeriodMillis, durationMillis int64) *companion.Timeline {
	t.Helper()
	timeline := &companion.Timeline{}
	var points []companion.Point
	for at := int64(0); at <= durationMillis; at += halfPeriodMillis {
		position := 0.0
		if (at/halfPeriodMillis)%2 == 1 {
			position = 100
		}
		points = append(points, companion.Point{TimeMillis: at, Position: position})
	}
	if err := timeline.Append(points); err != nil {
		t.Fatal(err)
	}
	return timeline
}

// settle runs the renderer past its fade-in and smoothing.
func settle(renderer *Renderer, timeline *companion.Timeline, until float64) float64 {
	drive := 0.0
	for at := 500.0; at <= until; at += 100 {
		drive, _ = renderer.Drive(timeline, at)
	}
	return drive
}

func TestFollowSpeedRisesWithStrokeSpeed(t *testing.T) {
	slow := settle(NewRenderer(Profile{Follow: FollowSpeed}), strokes(t, 1000, 10_000), 6000)
	fast := settle(NewRenderer(Profile{Follow: FollowSpeed}), strokes(t, 250, 10_000), 6000)
	if !(fast > slow && slow > 0) {
		t.Fatalf("drive slow %.3f fast %.3f; faster strokes must drive harder", slow, fast)
	}
}

func TestFollowSpeedDoesNotCancelAtAReversal(t *testing.T) {
	timeline := strokes(t, 250, 10_000)
	renderer := NewRenderer(Profile{Follow: FollowSpeed})
	renderer.Restart(-10_000)
	// 3000 ms is a turnaround: the window spans motion both ways.
	if drive, _ := renderer.Drive(timeline, 3000); drive < 0.3 {
		t.Fatalf("drive at a reversal = %.3f; averaged |velocity| must not cancel", drive)
	}
}

func TestFollowDepthIsStrongestAtTheChosenEnd(t *testing.T) {
	timeline := &companion.Timeline{}
	_ = timeline.Append([]companion.Point{{TimeMillis: 0, Position: 0}, {TimeMillis: 10_000, Position: 0}})
	base := NewRenderer(Profile{Follow: FollowDepth, DepthEnd: DepthBase})
	tip := NewRenderer(Profile{Follow: FollowDepth, DepthEnd: DepthTip})
	if b, tp := settle(base, timeline, 6000), settle(tip, timeline, 6000); b < 0.95 || tp != 0 {
		t.Fatalf("at the base: base end %.3f, tip end %.3f", b, tp)
	}
}

func TestSteadyHoldsWhileCoveredAndFadesIn(t *testing.T) {
	timeline := strokes(t, 500, 10_000)
	renderer := NewRenderer(Profile{Follow: FollowSteady})
	renderer.Restart(1000)
	half, _ := renderer.Drive(timeline, 2000)
	full, _ := renderer.Drive(timeline, 3500)
	if math.Abs(half-0.5) > 0.01 || full != 1 {
		t.Fatalf("fade-in half %.3f full %.3f", half, full)
	}
	renderer.Restart(5000)
	if restarted, _ := renderer.Drive(timeline, 5000); restarted != 0 {
		t.Fatalf("Resume must fade in again, got %.3f", restarted)
	}
}

func TestUncoveredTimeIsSilent(t *testing.T) {
	timeline := strokes(t, 500, 1000)
	renderer := NewRenderer(Profile{Follow: FollowSteady})
	renderer.Restart(-10_000)
	if drive, covered := renderer.Drive(timeline, 1500); covered || drive != 0 {
		t.Fatalf("past the frame: drive %.3f covered %v", drive, covered)
	}
}

func TestOutputMapsIntoTheLiveRange(t *testing.T) {
	if Output(0.5, 0.1, 0) != 0 {
		t.Fatal("zero intensity must be silent")
	}
	if Output(0, 0.1, 0.4) != 0 {
		t.Fatal("zero drive must be exactly zero, not the minimum")
	}
	if got := Output(0.5, 0.1, 0.5); math.Abs(got-0.3) > 1e-9 {
		t.Fatalf("Output(0.5, 0.1, 0.5) = %.3f, want 0.3", got)
	}
	if got := Output(1, 0.3, 0.2); got != 0.2 {
		t.Fatalf("a minimum above the intensity must not raise it: %.3f", got)
	}
}

func TestQuantizeHoldsAStepAndRoundsTheCeilingDown(t *testing.T) {
	if Quantize(0.36, 10, 3, 1) != 3 {
		t.Fatal("hysteresis must hold the previous step near a boundary")
	}
	if Quantize(0.40, 10, 3, 1) != 4 {
		t.Fatal("a clear change must move the step")
	}
	// One step cannot deliver 40%: the ceiling rounds down to silence.
	if Quantize(0.4, 1, -1, 0.4) != 0 {
		t.Fatal("the ceiling must quantize downward")
	}
	if Quantize(0, 20, 5, 1) != 0 {
		t.Fatal("zero is always exact")
	}
}
