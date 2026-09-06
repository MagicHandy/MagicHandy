package motion

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestReversalFilterMatchesOriginalRemovalOrder(t *testing.T) {
	random := rand.New(rand.NewPCG(17, 31)) // #nosec G404 -- reproducible test data.
	for trial := range 10_000 {
		points := make([]CurvePoint, 2+random.IntN(150))
		at := int64(0)
		for i := range points {
			points[i] = CurvePoint{TimeMillis: at, PositionPercent: float64(random.IntN(8))}
			if trial%3 == 1 && i > 0 {
				points[i].PositionPercent = math.Max(0, math.Min(100, points[i-1].PositionPercent+float64(random.IntN(7)-3)/2))
			} else if trial%3 == 2 && i%5 != 0 {
				points[i].PositionPercent = points[i-1].PositionPercent
			}
			at += int64(1 + random.IntN(400))
		}
		prominence := float64(random.IntN(8))
		original := slices.Clone(points)
		got := StabilizePatternReversals(points, prominence)
		want := referenceStabilizePatternReversals(points, prominence)
		if !slices.Equal(got, want) {
			t.Fatalf("trial %d prominence %v\ninput=%v\ngot=%v\nwant=%v", trial, prominence, points, got, want)
		}
		got[0].PositionPercent = 99
		if !slices.Equal(points, original) {
			t.Fatal("filter mutated or aliased authored points")
		}
	}
}

func TestReversalFilterHandlesMaximumMediaChatter(t *testing.T) {
	points := make([]CurvePoint, MaximumMediaTimelinePoints)
	for i := range points {
		points[i] = CurvePoint{TimeMillis: int64(i) * 100, PositionPercent: 50 + float64(i%2)}
	}
	got := StabilizePatternReversals(points, 3)
	// The original policy removes the high interior reversals and preserves
	// every intervening equal-position point plus the final rising endpoint.
	want := make([]CurvePoint, 0, len(points)/2+1)
	for i := 0; i < len(points); i += 2 {
		want = append(want, points[i])
	}
	want = append(want, points[len(points)-1])
	if !slices.Equal(got, want) {
		t.Fatalf("dense chatter retained %d points, want %d", len(got), len(want))
	}
}

// Frozen pre-optimization implementation: the incremental filter must emit
// exactly these points in exactly this order, including monotonic details and
// the last point of a plateau. This is an equivalence oracle, not a new policy.
func referenceStabilizePatternReversals(points []CurvePoint, minimumProminence float64) []CurvePoint {
	result := slices.Clone(points)
	if minimumProminence <= 0 {
		return result
	}
	for len(result) > 2 {
		anchors := curveReversalAnchors(result)
		removed := false
		for index := 1; index < len(anchors)-1; index++ {
			left, current, right := result[anchors[index-1]], result[anchors[index]], result[anchors[index+1]]
			prominence := math.Min(math.Abs(current.PositionPercent-left.PositionPercent), math.Abs(current.PositionPercent-right.PositionPercent))
			if prominence > minimumProminence || min(current.TimeMillis-left.TimeMillis, right.TimeMillis-current.TimeMillis) > patternChatterFlankMillis(result) {
				continue
			}
			pointIndex := anchors[index]
			result = append(result[:pointIndex], result[pointIndex+1:]...)
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	return result
}

func TestMediaNormalizationPreservesValidationAndOwnership(t *testing.T) {
	for _, points := range [][]CurvePoint{
		{{TimeMillis: 0, PositionPercent: 20}, {TimeMillis: 100, PositionPercent: 80}},
		{{TimeMillis: 100, PositionPercent: 80}, {TimeMillis: 0, PositionPercent: 20}},
		{{TimeMillis: 0, PositionPercent: -20}, {TimeMillis: 100, PositionPercent: 120}},
		{{TimeMillis: 0, PositionPercent: 20}, {TimeMillis: 0, PositionPercent: 30}, {TimeMillis: 100, PositionPercent: 80}},
		{{TimeMillis: 20, PositionPercent: 20}, {TimeMillis: 100, PositionPercent: 80}},
		{{TimeMillis: 0, PositionPercent: math.NaN()}, {TimeMillis: 100, PositionPercent: 80}},
	} {
		for _, duration := range []int64{0, 50, 100} {
			want, wantDuration, wantErr := normalizePointsWithLimit(points, duration, false, MaximumMediaTimelinePoints)
			if wantErr == nil {
				_, wantErr = newCurve(want, wantDuration, false, true, MaximumMediaTimelinePoints, neutralPlaybackScale())
			}
			got, err := NormalizeMediaTimelineDefinition(MediaTimelineDefinition{ID: "video", Name: "Video", Points: points, DurationMillis: duration})
			if (err != nil) != (wantErr != nil) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			if err != nil {
				continue
			}
			if got.DurationMillis != wantDuration || !slices.Equal(got.Points, want) {
				t.Fatalf("normalization changed: got %+v, want %v at %d", got, want, wantDuration)
			}
			first := points[0]
			got.Points[0].PositionPercent = 99
			if points[0] != first {
				t.Fatal("normalized points alias the caller's slice")
			}
		}
	}
}
