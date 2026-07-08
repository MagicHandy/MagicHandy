package motion

import (
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
