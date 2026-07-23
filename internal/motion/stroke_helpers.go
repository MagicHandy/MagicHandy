package motion

import (
	"math"
	"math/rand"
)

func clampDelta(atrasoMS int, hardwareSafetyLock bool) int {
	delta := atrasoMS
	if delta < 1 {
		delta = 1
	}
	if hardwareSafetyLock && delta < 30 {
		delta = 30
	}
	return delta
}

func positionsToTimedWaypoints(
	positions []int,
	physics ChaoticPhysics,
	atrasoMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	intensidade := physics.Intensidade
	if intensidade <= 0 {
		intensidade = 50
	}
	out := make([]ChaosWaypoint, len(positions))
	denom := len(positions) - 1
	if denom < 1 {
		denom = 1
	}
	for i, pos := range positions {
		progress := float64(i) / float64(denom)
		delta := jitteredTimeDelta(atrasoMS, progress, intensidade, physics, hardwareSafetyLock, rng)
		out[i] = ChaosWaypoint{
			TimeDelta: delta,
			Position:  pos,
		}
	}
	return out
}

func curvedPositions(from, to int, count int) []int {
	if count < 2 {
		count = 2
	}
	out := make([]int, count)
	for i := 0; i < count; i++ {
		t := float64(i) / float64(count-1)
		eased := easeInOutSine(t)
		out[i] = from + int(math.Round(float64(to-from)*eased))
	}
	return out
}

func easeInOutSine(t float64) float64 {
	return 0.5 - 0.5*math.Cos(math.Pi*t)
}

func absIntValue(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// proceduralStreamLeadMillis is the lead used when estimating stream duration in tests.
const proceduralStreamLeadMillis = organicStreamLeadMillis
