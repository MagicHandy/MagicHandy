package vacuglide

import (
	"math"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/companion"
)

// strokes builds full strokes between low and high, each half lasting half
// milliseconds, for duration milliseconds.
func strokes(low, high float64, half, duration int64) *companion.Timeline {
	timeline := &companion.Timeline{}
	var points []companion.Point
	for at, up := int64(0), true; at <= duration; at, up = at+half, !up {
		position := low
		if up {
			position = high
		}
		points = append(points, companion.Point{TimeMillis: at, Position: position})
	}
	_ = timeline.Append(points)
	return timeline
}

func TestSpeedFollowsPaceOnAutoblowsScale(t *testing.T) {
	// Full strokes, 0->100 in 500 ms: 200 percent per second, a third of the
	// 600 that is full motor speed.
	timeline := strokes(0, 100, 500, 10_000)
	renderer := NewRenderer(Profile{Scale: 1})
	var speed float64
	for at := 1000.0; at <= 5000; at += 100 {
		speed, _ = renderer.Speed(timeline, at)
	}
	if math.Abs(speed-100.0/3) > 1 {
		t.Fatalf("speed = %.1f%%, want about 33%%", speed)
	}
	full := NewRenderer(Profile{Scale: 2})
	for at := 1000.0; at <= 5000; at += 100 {
		speed, _ = full.Speed(timeline, at)
	}
	if math.Abs(speed-200.0/3) > 1 {
		t.Fatalf("scaled speed = %.1f%%, want about 67%%", speed)
	}
}

func TestShortStrokesAtThePaceStillDriveTheMotor(t *testing.T) {
	// Half-length strokes at twice the rate have the same pace.
	long := strokes(0, 100, 500, 10_000)
	short := strokes(40, 90, 250, 10_000)
	a, b := NewRenderer(Profile{Scale: 1}), NewRenderer(Profile{Scale: 1})
	var longSpeed, shortSpeed float64
	for at := 1000.0; at <= 5000; at += 100 {
		longSpeed, _ = a.Speed(long, at)
		shortSpeed, _ = b.Speed(short, at)
	}
	if math.Abs(longSpeed-shortSpeed) > 1.5 {
		t.Fatalf("long strokes %.1f%% and short strokes %.1f%% at the same pace differ", longSpeed, shortSpeed)
	}
}

func TestHoldsStopTheMotorAndSlowMovementStaysAboveTheStall(t *testing.T) {
	timeline := &companion.Timeline{}
	_ = timeline.Append([]companion.Point{{TimeMillis: 0, Position: 50}, {TimeMillis: 3000, Position: 50}, {TimeMillis: 9000, Position: 62}})
	renderer := NewRenderer(Profile{Scale: 1})
	if speed, ok := renderer.Speed(timeline, 2500); !ok || speed != 0 {
		t.Fatalf("held speed = %.1f, %v; want 0", speed, ok)
	}
	// 2 percent per second is about 0.3%: below the hold threshold, so stopped.
	if speed, _ := renderer.Speed(timeline, 8000); speed != 0 {
		t.Fatalf("a crawl below the hold threshold = %.1f, want 0", speed)
	}
	if got := MotorPercent(2.5); got != FloorPercent {
		t.Fatalf("MotorPercent(2.5) = %.1f, want the floor", got)
	}
	if got := MotorPercent(140); got != 100 {
		t.Fatalf("MotorPercent(140) = %.1f, want 100", got)
	}
	if got := MotorPercent(math.NaN()); got != 0 {
		t.Fatalf("MotorPercent(NaN) = %.1f, want 0", got)
	}
}

func TestSpeedReportsUncoveredTime(t *testing.T) {
	timeline := strokes(0, 100, 500, 2000)
	renderer := NewRenderer(Profile{Scale: 1})
	if _, ok := renderer.Speed(timeline, 2500); ok {
		t.Fatal("time past the frame must be uncovered")
	}
	if _, ok := renderer.Speed(&companion.Timeline{}, 0); ok {
		t.Fatal("an empty frame covers nothing")
	}
}

func TestReachReadsHowDeepTheStrokesGo(t *testing.T) {
	renderer := NewRenderer(Profile{Scale: 1})
	tip := strokes(70, 100, 400, 6000)
	if reach, ok := renderer.Reach(tip, 5000); !ok || math.Abs(reach-30) > 1e-9 {
		t.Fatalf("tip strokes reach %.1f, %v; want 30", reach, ok)
	}
	full := strokes(0, 100, 400, 6000)
	if reach, _ := renderer.Reach(full, 5000); math.Abs(reach-100) > 1e-9 {
		t.Fatalf("full strokes reach %.1f, want 100", reach)
	}
}
