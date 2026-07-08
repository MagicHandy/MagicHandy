package motion

import (
	"math"
	"math/rand"
)

// ChaosWaypoint is one generated timed step for procedural chaotic motion.
// TimeDelta is the time gap (ms) from the previous waypoint to this one.
// Position is expressed in semantic Handy 0..100 axis units.
type ChaosWaypoint struct {
	TimeDelta int
	Position  int
}

type regionRange struct {
	Min int
	Max int
}

func chaosRegionRange(region string) (regionRange, bool) {
	switch region {
	case "cabeca":
		return regionRange{Min: 70, Max: 100}, true
	case "meio":
		return regionRange{Min: 30, Max: 69}, true
	case "base":
		return regionRange{Min: 0, Max: 29}, true
	case "full", "completo", "cabeca_base":
		return regionRange{Min: 0, Max: 100}, true
	case "meio_cabeca":
		return regionRange{Min: 30, Max: 100}, true
	case "meio_base":
		return regionRange{Min: 0, Max: 69}, true
	case "aleatoria":
		return regionRange{Min: 0, Max: 100}, true
	default:
		return regionRange{}, false
	}
}

func clampInt(v, minVal, maxVal int) int {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func velocityToBaseDeltaMillis(velocity int) int {
	// Maps 0..100 velocity to a 35ms..16ms base delta range.
	velocity = clampInt(velocity, 0, 100)
	minDelta := 16.0
	maxDelta := 35.0
	f := float64(velocity) / 100.0
	// Higher velocity => smaller delta.
	d := maxDelta - f*(maxDelta-minDelta)
	return int(math.Round(d))
}

func easingShrinkFactor(progress01 float64, intensity int) float64 {
	// High intensity shrinks time deltas near the end.
	// intensity 0..100 => exponent 1..3.
	exp := 1.0 + float64(intensity)/50.0
	endFactor := 0.15 // final delta ~15% of base
	// (1-progress)^exp => 1 at start, 0 at end.
	shape := math.Pow(1.0-progress01, exp)
	return endFactor + (1.0-endFactor)*shape
}

func randomInRange(r regionRange, rng *rand.Rand) int {
	if r.Min >= r.Max {
		return r.Min
	}
	return r.Min + rng.Intn(r.Max-r.Min+1)
}

func generatePositionsSimple(r regionRange, rng *rand.Rand) []int {
	return []int{randomInRange(r, rng), randomInRange(r, rng)}
}

func generatePositionsLeve(r regionRange, velocity int, intensity int, rng *rand.Rand) []int {
	// 3 for slower, 4 for faster.
	count := 3
	if velocity >= 55 {
		count = 4
	}
	span := r.Max - r.Min
	pos := randomInRange(r, rng)
	out := make([]int, 0, count)
	out = append(out, pos)
	noise := float64(span) * 0.10 * (0.8 + float64(intensity)/250.0)
	for i := 1; i < count; i++ {
		sign := 1.0
		if rng.Float64() < 0.5 {
			sign = -1.0
		}
		step := sign * noise * (0.6 + rng.Float64()*0.8)
		pos = clampInt(int(math.Round(float64(pos)+step)), r.Min, r.Max)
		out = append(out, pos)
	}
	return out
}

func generatePositionsModerado(r regionRange, velocity int, intensity int, rng *rand.Rand) []int {
	// Density grows with velocity but stays bounded.
	count := 6 + velocity/20
	if count < 6 {
		count = 6
	}
	if count > 12 {
		count = 12
	}
	span := r.Max - r.Min
	pos := randomInRange(r, rng)
	out := make([]int, 0, count)
	out = append(out, pos)
	noise := float64(span) * 0.30 * (0.8 + float64(intensity)/250.0)
	for i := 1; i < count; i++ {
		// Oscillate around the previous position with larger noise.
		step := (rng.Float64()*2 - 1) * noise * (0.7 + rng.Float64()*0.7)
		pos = clampInt(int(math.Round(float64(pos)+step)), r.Min, r.Max)
		out = append(out, pos)
	}
	return out
}

func generatePositionsAlto(r regionRange, blocks int, rng *rand.Rand) []int {
	span := r.Max - r.Min
	// Blocks contribute oscillatory chaos.
	out := make([]int, 0, blocks*20)
	pos := randomInRange(r, rng)
	out = append(out, pos)
	for b := 0; b < blocks; b++ {
		microCount := 15 + rng.Intn(16) // 15..30
		// amplitude ~ 35% of region span.
		amp := float64(span) * 0.35
		for i := 0; i < microCount; i++ {
			step := (rng.Float64()*2 - 1) * amp * (0.6 + rng.Float64()*0.9)
			pos = clampInt(int(math.Round(float64(pos)+step)), r.Min, r.Max)
			out = append(out, pos)
		}
	}
	return out
}

// GenerateChaoticWaypoints builds timed chaotic procedural motion steps.
func GenerateChaoticWaypoints(
	velocity int,
	intensity int,
	region string,
	tipoBatida string,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	if rng == nil {
		// #nosec G404 -- deterministic fallback for unit-test friendliness.
		rng = rand.New(rand.NewSource(1))
	}
	r, ok := chaosRegionRange(region)
	if !ok {
		// Default to full range rather than failing motion.
		r = regionRange{Min: 0, Max: 100}
	}
	velocity = clampInt(velocity, 0, 100)
	intensity = clampInt(intensity, 0, 100)

	baseDelta := velocityToBaseDeltaMillis(velocity)

	var positions []int
	switch tipoBatida {
	case "simples":
		positions = generatePositionsSimple(r, rng)
	case "leve":
		positions = generatePositionsLeve(r, velocity, intensity, rng)
	case "moderado":
		positions = generatePositionsModerado(r, velocity, intensity, rng)
	case "alto":
		blocks := 2 + velocity/50 // 2..4
		if blocks < 2 {
			blocks = 2
		}
		if blocks > 4 {
			blocks = 4
		}
		positions = generatePositionsAlto(r, blocks, rng)
	default:
		positions = generatePositionsLeve(r, velocity, intensity, rng)
	}

	if len(positions) < 2 {
		positions = []int{randomInRange(r, rng), randomInRange(r, rng)}
	}

	// Jitter time deltas to emulate micro variance.
	jitterFrac := 0.0
	switch tipoBatida {
	case "leve":
		jitterFrac = 0.05
	case "moderado":
		jitterFrac = 0.12
	case "alto":
		jitterFrac = 0.25
	}

	out := make([]ChaosWaypoint, 0, len(positions))
	// First delta is emitted as part of the sequence; consumers treat it as
	// the time to the first step.
	for i, pos := range positions {
		progress := float64(i) / float64(len(positions)-1)
		factor := easingShrinkFactor(progress, intensity)
		deltaF := float64(baseDelta) * factor
		// +/- jitter (relative), biased by randomness.
		if jitterFrac > 0 {
			deltaF = deltaF * (1 + (rng.Float64()*2-1)*jitterFrac)
		}
		delta := int(math.Round(deltaF))
		if delta < 1 {
			delta = 1
		}
		// Hardware safety clamp is applied after full generation.
		if hardwareSafetyLock && delta < 30 {
			delta = 30
		}
		out = append(out, ChaosWaypoint{TimeDelta: delta, Position: clampInt(pos, r.Min, r.Max)})
	}
	return out
}
