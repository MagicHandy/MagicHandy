package motion

import (
	"math"
	"math/rand"
)

// StrokeProfile defines asymmetric timing and bottom bounce for one stroke cycle.
type StrokeProfile struct {
	DownstrokeRatio float64 // fraction of cycle spent descending (e.g. 0.35)
	UpstrokeRatio   float64 // fraction of cycle spent ascending (e.g. 0.65)
	HasBottomBounce bool    // inject gag/bounce micro-keyframes at the bottom
}

// DefaultStrokeProfile is symmetric with no bounce.
func DefaultStrokeProfile() StrokeProfile {
	return StrokeProfile{
		DownstrokeRatio: 0.5,
		UpstrokeRatio:   0.5,
	}
}

// NormalizeStrokeProfile clamps ratios and ensures they sum to 1.
func NormalizeStrokeProfile(profile StrokeProfile) StrokeProfile {
	down := clampFloat(profile.DownstrokeRatio, 0.1, 0.9)
	up := clampFloat(profile.UpstrokeRatio, 0.1, 0.9)
	total := down + up
	if total <= 0 {
		return DefaultStrokeProfile()
	}
	profile.DownstrokeRatio = down / total
	profile.UpstrokeRatio = up / total
	return profile
}

// StrokeProfileFromAction maps director action names to stroke signatures.
func StrokeProfileFromAction(action string) StrokeProfile {
	switch action {
	case "riding":
		return NormalizeStrokeProfile(StrokeProfile{
			DownstrokeRatio: 0.35,
			UpstrokeRatio:   0.65,
		})
	case "deepthroat":
		return NormalizeStrokeProfile(StrokeProfile{
			DownstrokeRatio: 0.30,
			UpstrokeRatio:   0.70,
			HasBottomBounce: true,
		})
	default:
		return DefaultStrokeProfile()
	}
}

// buildBottomBounce inserts gag micro-keyframes at the stroke bottom (normalized 0..1).
func buildBottomBounce(
	botPosPct float64,
	strokeMinPct, strokeMaxPct float64,
	perlin *PerlinNoise,
	phase float64,
	rng *rand.Rand,
) []ChaosWaypoint {
	span := strokeMaxPct - strokeMinPct
	if span <= 0 {
		span = 100
	}
	wobble := perlin.Noise2D(phase*3.17, 88.4) * 0.05
	if rng != nil {
		wobble += (rng.Float64()*2 - 1) * 0.03
	}
	liftNorm := clampFloat(0.15+wobble, 0.08, 0.22)

	botNorm := botPosPct / 100.0
	minNorm := strokeMinPct / 100.0
	maxNorm := strokeMaxPct / 100.0
	liftNormPos := clampFloat(botNorm+liftNorm*(span/100.0), minNorm, maxNorm)

	bounceMS := 75
	if rng != nil {
		bounceMS += int(math.Round((rng.Float64()*2 - 1) * 12))
	}
	if bounceMS < 55 {
		bounceMS = 55
	}

	liftPct := int(math.Round(liftNormPos * 100))
	botPct := int(math.Round(clampFloat(botPosPct, strokeMinPct, strokeMaxPct)))

	return []ChaosWaypoint{
		{TimeDelta: bounceMS, Position: liftPct},
		{TimeDelta: bounceMS, Position: botPct},
	}
}
