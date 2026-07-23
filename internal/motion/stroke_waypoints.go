package motion

import (
	"math"
	"math/rand"
)

// GenerateStrokeWaypoints builds zone-aware organic Perlin strokes.
func GenerateStrokeWaypoints(
	physics ChaoticPhysics,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	return generateOrganicFromPhysics(physics, durationMS, hardwareSafetyLock, rng, -1)
}

// GenerateStrokeWaypointsFromPosition continues motion from a device position (0..100).
func GenerateStrokeWaypointsFromPosition(
	physics ChaoticPhysics,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	return generateOrganicFromPhysics(physics, durationMS, hardwareSafetyLock, rng, continueFrom)
}

// GenerateProceduralStream fills a segment with one continuous organic wave.
func GenerateProceduralStream(
	physics ChaoticPhysics,
	durationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	return generateOrganicStreamFromPhysics(physics, durationMillis, hardwareSafetyLock, rng, -1)
}

// GenerateProceduralStreamFromPosition continues freestyle motion from a device position.
func GenerateProceduralStreamFromPosition(
	physics ChaoticPhysics,
	durationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	return generateOrganicStreamFromPhysics(physics, durationMillis, hardwareSafetyLock, rng, continueFrom)
}

func generateOrganicFromPhysics(
	physics ChaoticPhysics,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	if durationMS <= 0 {
		durationMS = EstimateChatMotionDurationMS(physics)
	}
	if IsTurboTipo(physics.TipoBatida) {
		return GenerateTurboWaypointsForDuration(physics, durationMS, hardwareSafetyLock, rng, continueFrom)
	}
	cfg := OrganicConfigFromPhysics(physics)
	stream := GenerateOrganicWaypointsFromPosition(cfg, durationMS, hardwareSafetyLock, rng, continueFrom)
	return stream
}

func generateOrganicStreamFromPhysics(
	physics ChaoticPhysics,
	durationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	if durationMillis <= 0 {
		return generateOrganicFromPhysics(physics, 0, hardwareSafetyLock, rng, continueFrom)
	}
	if IsTurboTipo(physics.TipoBatida) {
		return GenerateTurboWaypointsForDuration(physics, int(durationMillis), hardwareSafetyLock, rng, continueFrom)
	}
	cfg := OrganicConfigFromPhysics(physics)
	stream := GenerateOrganicStreamFromPosition(cfg, durationMillis, hardwareSafetyLock, rng, continueFrom)
	return stream
}

// GenerateProceduralStreamFromBlend crossfades from live device state into a new organic stream.
func GenerateProceduralStreamFromBlend(
	physics ChaoticPhysics,
	targetDurationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	from MotionBlendState,
) []ChaosWaypoint {
	continueFrom := int(math.Round(from.Position))
	if IsTurboTipo(physics.TipoBatida) {
		// Hermite crossfade destroys turbo zigzag — splice buzz directly from playhead.
		return generateOrganicStreamFromPhysics(physics, targetDurationMillis, hardwareSafetyLock, rng, continueFrom)
	}
	stream := generateOrganicStreamFromPhysics(physics, targetDurationMillis, hardwareSafetyLock, rng, -1)
	opts := CrossfadeOptionsForPhysics(physics)
	opts.HardwareSafetyLock = hardwareSafetyLock
	return StitchWithCrossfade(from, stream, opts, rng)
}

// EstimateChatMotionDurationMS picks a single chat dispatch duration from physics.
func EstimateChatMotionDurationMS(physics ChaoticPhysics) int {
	switch normalizeTipoBatida(physics.TipoBatida) {
	case "very_fast", "vibrate", "turbo":
		return 2_500
	case "alto":
		return 4_000
	case "moderado":
		return 5_000
	case "leve", "simples":
		return 6_000
	default:
		return 7_000
	}
}

// ResolveAtrasoMS returns the smoothing delay between stroke points.
func ResolveAtrasoMS(physics ChaoticPhysics) int {
	var base int
	if physics.AtrasoMS > 0 {
		base = clampInt(physics.AtrasoMS, 1, 500)
	} else {
		base = tipoBaseAtrasoMS(physics.TipoBatida)
	}
	return scaleAtrasoByVelocity(base, physics.Velocidade)
}

func normalizeTipoBatida(value string) string {
	switch value {
	case "simples", "leve", "moderado", "alto", "fluido", "lento", "very_fast", "vibrate", "turbo":
		return value
	default:
		return "fluido"
	}
}
