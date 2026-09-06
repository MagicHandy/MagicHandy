package media

import (
	"slices"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestNoFiltersLeavesTheScriptUntouched(t *testing.T) {
	authored := []motion.CurvePoint{{TimeMillis: 0, PositionPercent: 10}, {TimeMillis: 1000, PositionPercent: 90}, {TimeMillis: 2000, PositionPercent: 10}}
	filtered, effect := (Filters{}).apply(authored)
	if !slices.Equal(filtered, authored) || effect != (Effect{}) {
		t.Fatalf("zero filters changed the script: %v, %+v", filtered, effect)
	}
}

func TestMediaSmoothingPreservesMajorReversalBesideChatter(t *testing.T) {
	points := []motion.CurvePoint{
		{TimeMillis: 0, PositionPercent: 0}, {TimeMillis: 1000, PositionPercent: 100},
		{TimeMillis: 1040, PositionPercent: 98}, {TimeMillis: 1080, PositionPercent: 100},
		{TimeMillis: 2000, PositionPercent: 0},
	}
	filtered, effect := (Filters{SmoothingPercent: 3}).apply(points)
	want := []motion.CurvePoint{points[0], points[1], points[3], points[4]}
	if !slices.Equal(filtered, want) || effect.ActionsRemoved != 1 {
		t.Fatalf("filter = %v, effect %+v; want major peak and plateau preserved", filtered, effect)
	}
}

func TestMediaSmoothingPreservesSlowSubtleExcursions(t *testing.T) {
	for _, times := range [][3]int64{{0, 1000, 2000}, {0, 1000, 1040}, {0, 40, 1040}} {
		points := []motion.CurvePoint{{TimeMillis: times[0], PositionPercent: 50}, {TimeMillis: times[1], PositionPercent: 52}, {TimeMillis: times[2], PositionPercent: 50}}
		got, effect := (Filters{SmoothingPercent: 3}).apply(points)
		if !slices.Equal(got, points) || effect.ActionsRemoved != 0 {
			t.Fatalf("slow authored excursion removed: %v", points)
		}
	}
}

func TestMediaSmoothingUsesFixedLocalWindow(t *testing.T) {
	points := []motion.CurvePoint{{TimeMillis: 0, PositionPercent: 50}, {TimeMillis: 200, PositionPercent: 52}, {TimeMillis: 400, PositionPercent: 50}, {TimeMillis: 600, PositionPercent: 100}, {TimeMillis: 1000, PositionPercent: 0}}
	short, _ := (Filters{SmoothingPercent: 3}).apply(points)
	long, _ := (Filters{SmoothingPercent: 3}).apply(append(slices.Clone(points), motion.CurvePoint{TimeMillis: 100_000, PositionPercent: 100}))
	if len(short) != 4 || !slices.Equal(short, long[:len(long)-1]) {
		t.Fatalf("unrelated tail changed jitter removal: short=%v long=%v", short, long)
	}
}

func TestRoundingIsAnEnginePolicyInsteadOfExtraSourcePoints(t *testing.T) {
	script := Funscript{VideoID: "test", Name: "Test", DurationMillis: 2000, Actions: []FunscriptAction{{AtMillis: 0, Position: 10}, {AtMillis: 1000, Position: 90}, {AtMillis: 2000, Position: 10}}}
	plain, _, err := script.TimelineFrom(0, 1, Filters{})
	if err != nil {
		t.Fatal(err)
	}
	rounded, effect, err := script.TimelineFrom(0, 1, Filters{PeakRoundingMillis: 60})
	if err != nil {
		t.Fatal(err)
	}
	if rounded.RoundingMillis != 60 || !slices.Equal(rounded.Points, plain.Points) || effect != (Effect{}) {
		t.Fatalf("rounding changed source points: %+v", rounded)
	}
}
