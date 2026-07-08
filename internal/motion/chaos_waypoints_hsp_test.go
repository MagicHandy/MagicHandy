package motion

import (
	"math/rand"
	"testing"
)

func TestChaosWaypointsToTimedPointsAccumulatesLeadAndDeltas(t *testing.T) {
	// #nosec G404 -- deterministic RNG for unit tests.
	rng := rand.New(rand.NewSource(7))
	waypoints := GenerateChaoticWaypoints(80, 60, "meio", "leve", true, rng)
	points := ChaosWaypointsToTimedPoints(waypoints, 300)
	if len(points) != len(waypoints) {
		t.Fatalf("len = %d, want %d", len(points), len(waypoints))
	}
	if points[0].TimeMillis != 300 {
		t.Fatalf("first point time = %d, want 300", points[0].TimeMillis)
	}
	for index := 1; index < len(points); index++ {
		expected := points[index-1].TimeMillis + int64(waypoints[index].TimeDelta)
		if points[index].TimeMillis != expected {
			t.Fatalf("point[%d].TimeMillis = %d, want %d", index, points[index].TimeMillis, expected)
		}
	}
}
