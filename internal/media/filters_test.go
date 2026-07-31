package media

import (
	"math"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func triangle(peaks int, legMillis int64) []motion.CurvePoint {
	points := make([]motion.CurvePoint, 0, peaks*2+1)
	for index := 0; index <= peaks*2; index++ {
		position := 0.0
		if index%2 == 1 {
			position = 100
		}
		points = append(points, motion.CurvePoint{TimeMillis: int64(index) * legMillis, PositionPercent: position})
	}
	return points
}

// The whole point of rounding is that it asks less of the device than the
// corner it replaces: the straight body keeps its authored slope, so the
// fastest moment of the stroke is no faster than before.
func TestRoundPeaksNeverRaisesPeakVelocity(t *testing.T) {
	authored := triangle(6, 500)
	rounded, reduction := roundPeaks(authored, 100, MaxMediaFunscriptActions)

	if reduction <= 0 {
		t.Fatal("rounding reported no peak reduction; a rounded corner is necessarily cut")
	}
	if fastest(rounded) > fastest(authored)+1e-6 {
		t.Fatalf("rounded peak velocity %.4f exceeds authored %.4f", fastest(rounded), fastest(authored))
	}
	for index := 1; index < len(rounded); index++ {
		if rounded[index].TimeMillis <= rounded[index-1].TimeMillis {
			t.Fatalf("rounded times are not strictly increasing at %d", index)
		}
	}
	if bounds := span(rounded); bounds < span(authored)-2*reduction-0.001 {
		t.Fatalf("rounded span %.3f lost more than the reported %.3f per end", bounds, reduction)
	}
}

// A corner cannot be rounded without cutting it, so the reduction is real and
// has to be reported honestly. It must also stay proportional to the window.
func TestRoundPeaksReductionGrowsWithTheWindow(t *testing.T) {
	authored := triangle(4, 500)
	_, small := roundPeaks(authored, 20, MaxMediaFunscriptActions)
	_, large := roundPeaks(authored, 150, MaxMediaFunscriptActions)
	if !(small > 0 && large > small) {
		t.Fatalf("reduction did not grow with the window: %.3f then %.3f", small, large)
	}
	if large > 50 {
		t.Fatalf("reduction %.3f is more than half a full-range stroke", large)
	}
}

func TestFilletCornerReportsTheActualRoundedApexReduction(t *testing.T) {
	knots, reduction := filletCorner(
		motion.CurvePoint{TimeMillis: 0, PositionPercent: 0},
		motion.CurvePoint{TimeMillis: 500, PositionPercent: 100},
		motion.CurvePoint{TimeMillis: 1000, PositionPercent: 0},
		100,
		3,
	)
	if len(knots) != 5 {
		t.Fatalf("fillet knot count = %d, want 5", len(knots))
	}
	if math.Abs(reduction-10) > 1e-9 {
		t.Fatalf("reported apex reduction = %.3f, want 10", reduction)
	}
	highest := 0.0
	for _, knot := range knots {
		highest = math.Max(highest, knot.PositionPercent)
	}
	if math.Abs(reduction-(100-highest)) > 1e-9 {
		t.Fatalf("reported reduction %.3f does not match emitted apex %.3f", reduction, highest)
	}
}

// Rounding must never consume the leg between two corners, or a fast script
// would lose its shape entirely rather than its corners.
func TestRoundPeaksIsCappedByItsOwnLeg(t *testing.T) {
	// 40ms legs against a 200ms request: the per-corner cap has to win.
	authored := triangle(6, 40)
	rounded, _ := roundPeaks(authored, 200, MaxMediaFunscriptActions)
	for index := 1; index < len(rounded); index++ {
		if rounded[index].TimeMillis <= rounded[index-1].TimeMillis {
			t.Fatalf("times collapsed at %d: %+v", index, rounded[index-1:index+1])
		}
	}
	if fastest(rounded) > fastest(authored)+1e-6 {
		t.Fatal("a capped fillet still raised peak velocity")
	}
}

// A script dense enough that fillets would not fit plays unrounded. Thinning it
// to fit would silently change motion the user did not ask to change.
func TestRoundPeaksDeclinesRatherThanExceedingThePointBudget(t *testing.T) {
	authored := triangle(20, 100)
	rounded, reduction := roundPeaks(authored, 30, len(authored))
	if len(rounded) != len(authored) || reduction != 0 {
		t.Fatalf("rounding ran inside a full point budget: %d points, %.3f reduction", len(rounded), reduction)
	}
}

// The zero value is authored-exact. This is the rule every filter default
// depends on, so it is pinned rather than assumed.
func TestNoFiltersLeavesTheScriptUntouched(t *testing.T) {
	authored := triangle(5, 400)
	filtered, effect := Filters{}.apply(authored, MaxMediaFunscriptActions)
	if len(filtered) != len(authored) || effect != (Effect{}) {
		t.Fatalf("zero-value filters changed the script: %d points, %+v", len(filtered), effect)
	}
}

// Smoothing removes spikes and reports how many, so an unhelpful setting is
// visible instead of mysterious.
func TestSmoothingRemovesSpikesAndReportsTheCount(t *testing.T) {
	// The chatter window scales with the script, capped at 250ms, so the
	// fixture is long enough to exercise the cap a real media slice would hit.
	points := []motion.CurvePoint{
		{TimeMillis: 0, PositionPercent: 0},
		{TimeMillis: 400, PositionPercent: 100},
		{TimeMillis: 440, PositionPercent: 98},
		{TimeMillis: 480, PositionPercent: 100},
		{TimeMillis: 900, PositionPercent: 0},
		{TimeMillis: 20_000, PositionPercent: 100},
		{TimeMillis: 40_000, PositionPercent: 0},
	}
	filtered, effect := Filters{SmoothingPercent: 3}.apply(points, MaxMediaFunscriptActions)
	if effect.ActionsRemoved == 0 {
		t.Fatal("smoothing removed nothing from a script with a 2% spike")
	}
	if len(filtered) != len(points)-effect.ActionsRemoved {
		t.Fatalf("reported %d removed but %d points remain of %d",
			effect.ActionsRemoved, len(filtered), len(points))
	}
}

func fastest(points []motion.CurvePoint) float64 {
	peak := 0.0
	for index := 1; index < len(points); index++ {
		span := float64(points[index].TimeMillis - points[index-1].TimeMillis)
		if span <= 0 {
			continue
		}
		peak = math.Max(peak, math.Abs(points[index].PositionPercent-points[index-1].PositionPercent)/span)
	}
	return peak
}

func span(points []motion.CurvePoint) float64 {
	low, high := points[0].PositionPercent, points[0].PositionPercent
	for _, point := range points {
		low = math.Min(low, point.PositionPercent)
		high = math.Max(high, point.PositionPercent)
	}
	return high - low
}
