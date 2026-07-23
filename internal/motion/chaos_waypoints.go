package motion

import (
	"math"
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

// RegionBounds maps a procedural regiao string to semantic stroke bounds (0..100).
func RegionBounds(regiao string) (min, max int, ok bool) {
	r, ok := chaosRegionRange(regiao)
	return r.Min, r.Max, ok
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

// effectiveSemanticRegion returns HSP axis bounds for waypoint generation.
// Regiao is the primary source; optional scene-director StrokeRange intersects.
// Device stroke window is applied at transport only (HSP invariant 2).
func effectiveSemanticRegion(physics ChaoticPhysics) regionRange {
	region, ok := chaosRegionRange(physics.Regiao)
	if !ok {
		region = regionRange{Min: 0, Max: 100}
	}
	if physics.StrokeRangeMax <= physics.StrokeRangeMin {
		return region
	}
	strokeMin := int(math.Round(physics.StrokeRangeMin * 100))
	strokeMax := int(math.Round(physics.StrokeRangeMax * 100))
	intersected := regionRange{
		Min: maxInt(region.Min, strokeMin),
		Max: minInt(region.Max, strokeMax),
	}
	if intersected.Max > intersected.Min {
		return intersected
	}
	return region
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
