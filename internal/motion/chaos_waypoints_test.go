package motion

import (
	"math"
	"math/rand"
	"testing"
)

func TestGenerateChaoticWaypointsRegionMapping(t *testing.T) {
	// #nosec G404 -- deterministic RNG for unit tests.
	rng := rand.New(rand.NewSource(1))
	wps := GenerateChaoticWaypoints(50, 50, "cabeca", "simples", true, rng)
	if len(wps) != 2 {
		t.Fatalf("len = %d, want 2", len(wps))
	}
	for _, wp := range wps {
		if wp.Position < 70 || wp.Position > 100 {
			t.Fatalf("position = %d, want within 70..100", wp.Position)
		}
	}
}

func TestGenerateChaoticWaypointsSafetyLockEnforcesMinDelta(t *testing.T) {
	// #nosec G404 -- deterministic RNG for unit tests.
	rng := rand.New(rand.NewSource(123))
	wps := GenerateChaoticWaypoints(100, 100, "aleatoria", "alto", true, rng)
	if len(wps) < 2 {
		t.Fatalf("len = %d, want >=2", len(wps))
	}
	for i, wp := range wps {
		if wp.TimeDelta < 30 {
			t.Fatalf("wp[%d].TimeDelta = %d, want >=30", i, wp.TimeDelta)
		}
	}
}

func TestGenerateChaoticWaypointsSafetyLockCanBeDisabled(t *testing.T) {
	// #nosec G404 -- deterministic RNG for unit tests.
	rng := rand.New(rand.NewSource(123))
	wps := GenerateChaoticWaypoints(100, 100, "aleatoria", "alto", false, rng)
	if len(wps) < 2 {
		t.Fatalf("len = %d, want >=2", len(wps))
	}
	foundUnder30 := false
	for _, wp := range wps {
		if wp.TimeDelta < 30 {
			foundUnder30 = true
			break
		}
	}
	if !foundUnder30 {
		t.Fatalf("expected at least one TimeDelta < 30 when safety lock is disabled")
	}
}

func TestGenerateChaoticWaypointsCounts(t *testing.T) {
	// #nosec G404 -- deterministic RNG for unit tests.
	rng := rand.New(rand.NewSource(2))

	simples := GenerateChaoticWaypoints(10, 10, "meio", "simples", true, rng)
	if len(simples) != 2 {
		t.Fatalf("simples len = %d, want 2", len(simples))
	}

	leve := GenerateChaoticWaypoints(60, 40, "meio", "leve", true, rng)
	if len(leve) != 4 {
		t.Fatalf("leve len = %d, want 4 (velocity>=55)", len(leve))
	}

	moderado := GenerateChaoticWaypoints(80, 40, "meio", "moderado", true, rng)
	if len(moderado) < 6 {
		t.Fatalf("moderado len = %d, want >=6", len(moderado))
	}

	alto := GenerateChaoticWaypoints(80, 40, "meio", "alto", true, rng)
	if len(alto) < 30 {
		t.Fatalf("alto len = %d, want >=30", len(alto))
	}
}

// --- Legacy generator (pre-organic stroke path). Test reference only; not used in production. ---

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
		step := (rng.Float64()*2 - 1) * noise * (0.7 + rng.Float64()*0.7)
		pos = clampInt(int(math.Round(float64(pos)+step)), r.Min, r.Max)
		out = append(out, pos)
	}
	return out
}

func generatePositionsAlto(r regionRange, blocks int, rng *rand.Rand) []int {
	span := r.Max - r.Min
	out := make([]int, 0, blocks*20)
	pos := randomInRange(r, rng)
	out = append(out, pos)
	for b := 0; b < blocks; b++ {
		microCount := 15 + rng.Intn(16)
		amp := float64(span) * 0.35
		for i := 0; i < microCount; i++ {
			step := (rng.Float64()*2 - 1) * amp * (0.6 + rng.Float64()*0.9)
			pos = clampInt(int(math.Round(float64(pos)+step)), r.Min, r.Max)
			out = append(out, pos)
		}
	}
	return out
}

// GenerateChaoticWaypoints is retained for regression tests of the legacy tipo_batida generator.
func GenerateChaoticWaypoints(
	velocity int,
	intensity int,
	region string,
	tipoBatida string,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	r, ok := chaosRegionRange(region)
	if !ok {
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
		blocks := 2 + velocity/50
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
	for i, pos := range positions {
		progress := float64(i) / float64(len(positions)-1)
		factor := easingShrinkFactor(progress, intensity)
		deltaF := float64(baseDelta) * factor
		if jitterFrac > 0 {
			deltaF = deltaF * (1 + (rng.Float64()*2-1)*jitterFrac)
		}
		delta := int(math.Round(deltaF))
		if delta < 1 {
			delta = 1
		}
		if hardwareSafetyLock && delta < 30 {
			delta = 30
		}
		out = append(out, ChaosWaypoint{TimeDelta: delta, Position: clampInt(pos, r.Min, r.Max)})
	}
	return out
}
