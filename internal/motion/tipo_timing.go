package motion

import "math"

// tipoBaseAtrasoMS returns the default inter-point delay for each stroke profile.
// Values form a smooth ladder from lento → alto (very_fast uses the turbo sweep path).
func tipoBaseAtrasoMS(tipo string) int {
	switch normalizeTipoBatida(tipo) {
	case "lento":
		return 200
	case "fluido":
		return 168
	case "simples":
		return 138
	case "leve":
		return 118
	case "moderado":
		return 92
	case "alto":
		return 68
	case "very_fast":
		return 38
	case "vibrate", "turbo":
		return 1
	default:
		return 168
	}
}

// scaleAtrasoByVelocity applies a gentle velocity modifier to organic pacing.
func scaleAtrasoByVelocity(atrasoMS, velocidade int) int {
	if atrasoMS <= 1 {
		return atrasoMS
	}
	velocidade = clampInt(velocidade, 1, 100)
	// vel 25 ≈ 1.28× slower, vel 50 ≈ 1.0×, vel 100 ≈ 0.55× faster
	factor := 1.28 - float64(velocidade-25)*0.00973
	factor = clampFloat(factor, 0.52, 1.35)
	return clampInt(int(math.Round(float64(atrasoMS)*factor)), 1, 500)
}

// organicCycleVelocityScale tweaks stroke cycle length per tipo (1.0 = neutral).
func organicCycleVelocityScale(tipo string) float64 {
	switch normalizeTipoBatida(tipo) {
	case "lento":
		return 1.18
	case "fluido":
		return 1.05
	case "simples":
		return 1.0
	case "leve":
		return 0.96
	case "moderado":
		return 0.90
	case "alto":
		return 0.84
	default:
		return 1.0
	}
}
