package motion

import (
	"math"
	"math/rand"
	"testing"
)

func TestOrganicMotionHasTimingVariance(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:  50,
		Intensidade: 55,
		Regiao:      "meio_cabeca",
		TipoBatida:  "fluido",
		AtrasoMS:    160,
	}
	stream := GenerateStrokeWaypoints(physics, 4_000, false, rand.New(rand.NewSource(42)))
	stats := organicMotionStats(stream)
	stddev, ok := stats["delta_stddev"].(int)
	if !ok || stddev < 8 {
		t.Fatalf("delta_stddev = %v, want >= 8 for organic pacing", stats["delta_stddev"])
	}
	maxDelta, _ := stats["delta_max"].(int)
	minDelta, _ := stats["delta_min"].(int)
	if maxDelta > 0 && float64(maxDelta) < float64(minDelta)*1.15 {
		t.Fatalf("delta range too flat: min=%d max=%d", minDelta, maxDelta)
	}
}

func TestOrganicStrokeDepthVaries(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	seenShort := false
	seenFull := false
	for i := 0; i < 40; i++ {
		depth := organicStrokeDepth(rng)
		if depth < 0.82 {
			seenShort = true
		}
		if depth > 0.95 {
			seenFull = true
		}
	}
	if !seenShort || !seenFull {
		t.Fatalf("depth spread missing: short=%v full=%v", seenShort, seenFull)
	}
}

func TestOrganicCurvedPositionsWobbleWithinZone(t *testing.T) {
	region := regionRange{Min: 70, Max: 100}
	rng := rand.New(rand.NewSource(9))
	for i := 0; i < 30; i++ {
		positions := organicCurvedPositions(72, 98, 6, region, rng)
		for _, pos := range positions {
			if pos < region.Min || pos > region.Max {
				t.Fatalf("position %d outside %d..%d", pos, region.Min, region.Max)
			}
		}
	}
}

func TestJitteredTimeDeltaSpread(t *testing.T) {
	physics := ChaoticPhysics{TipoBatida: "moderado", Intensidade: 60}
	rng := rand.New(rand.NewSource(3))
	var deltas []int
	for i := 0; i < 40; i++ {
		deltas = append(deltas, jitteredTimeDelta(80, float64(i)/39, 60, physics, false, rng))
	}
	avg := 0.0
	for _, d := range deltas {
		avg += float64(d)
	}
	avg /= float64(len(deltas))
	variance := 0.0
	for _, d := range deltas {
		diff := float64(d) - avg
		variance += diff * diff
	}
	stddev := math.Sqrt(variance / float64(len(deltas)))
	if stddev < 6 {
		t.Fatalf("jitter stddev = %.2f, want human-like spread", stddev)
	}
}
