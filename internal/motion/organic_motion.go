package motion

import (
	"math"
	"math/rand"
)

func deltaJitterFraction(tipo string) float64 {
	switch normalizeTipoBatida(tipo) {
	case "fluido", "lento", "simples":
		return 0.24
	case "leve":
		return 0.20
	case "moderado":
		return 0.30
	case "alto":
		return 0.38
	case "very_fast", "vibrate", "turbo":
		return 0.18
	default:
		return 0.22
	}
}

func easeOutCubic(t float64) float64 {
	inv := 1 - t
	return 1 - inv*inv*inv
}

func easeInQuad(t float64) float64 {
	return t * t
}

func organicCurvedPositions(from, to int, count int, region regionRange, rng *rand.Rand) []int {
	if count < 2 {
		count = 2
	}
	if rng == nil {
		return curvedPositions(from, to, count)
	}

	profile := rng.Intn(4)
	wobble := float64(region.Max-region.Min) * (0.015 + rng.Float64()*0.035)
	out := make([]int, count)
	for i := 0; i < count; i++ {
		t := float64(i) / float64(count-1)
		var eased float64
		switch profile {
		case 0:
			eased = easeInOutSine(t)
		case 1:
			eased = easeOutCubic(t)
		case 2:
			eased = 0.35*easeInQuad(t) + 0.65*easeOutCubic(t)
		default:
			eased = 0.5*easeInOutSine(t) + 0.5*t
		}
		pos := from + int(math.Round(float64(to-from)*eased))
		if i > 0 && i < count-1 {
			pos += int(math.Round((rng.Float64()*2 - 1) * wobble))
			step := absIntValue(pos - out[i-1])
			if step > 0 && step < 3 {
				if pos >= out[i-1] {
					pos = clampInt(out[i-1]+3, region.Min, region.Max)
				} else {
					pos = clampInt(out[i-1]-3, region.Min, region.Max)
				}
			}
		}
		out[i] = clampInt(pos, region.Min, region.Max)
	}
	return out
}

func teaseWiggleAt(anchor int, region regionRange, rng *rand.Rand) []int {
	if rng == nil {
		return nil
	}
	span := region.Max - region.Min
	if span < 6 {
		return nil
	}
	amp := 3 + rng.Intn(3)
	count := 2 + rng.Intn(3)
	out := make([]int, 0, count)
	pos := anchor
	for i := 0; i < count; i++ {
		sign := 1
		if i%2 == 1 {
			sign = -1
		}
		pos = clampInt(anchor+sign*amp, region.Min, region.Max)
		out = append(out, pos)
	}
	return out
}

func organicReturnPosition(start, low, high int, rng *rand.Rand) int {
	if rng == nil {
		return start
	}
	offset := rng.Intn(9) - 4
	return clampInt(start+offset, low, high)
}

func organicStrokeDepth(rng *rand.Rand) float64 {
	if rng == nil {
		return 1.0
	}
	return 0.65 + rng.Float64()*0.35
}

func jitteredTimeDelta(
	baseMS int,
	progress float64,
	intensidade int,
	physics ChaoticPhysics,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) int {
	if baseMS <= 1 {
		return clampDelta(1, hardwareSafetyLock)
	}
	factor := easingShrinkFactor(progress, intensidade)
	if rng != nil && rng.Float64() < 0.45 {
		factor = 0.72 + 0.28*factor
	}
	deltaF := float64(baseMS) * factor
	if rng != nil {
		jitter := deltaJitterFraction(physics.TipoBatida)
		deltaF *= 1 + (rng.Float64()*2-1)*jitter
		roll := rng.Float64()
		switch {
		case roll < 0.07:
			deltaF *= 1.12 + rng.Float64()*0.38
		case roll < 0.12:
			deltaF *= 0.55 + rng.Float64()*0.25
		}
	}
	delta := clampDelta(int(math.Round(deltaF)), hardwareSafetyLock)
	if baseMS > 1 && delta > baseMS*2 {
		delta = baseMS * 2
	}
	return delta
}

func organicMotionStats(waypoints []ChaosWaypoint) map[string]any {
	if len(waypoints) < 2 {
		return map[string]any{"point_count": len(waypoints)}
	}
	var sum, minDelta, maxDelta float64
	minDelta = float64(waypoints[1].TimeDelta)
	maxDelta = minDelta
	reversals := 0
	for i := 1; i < len(waypoints); i++ {
		d := float64(waypoints[i].TimeDelta)
		sum += d
		if d < minDelta {
			minDelta = d
		}
		if d > maxDelta {
			maxDelta = d
		}
		if i >= 2 {
			prev := waypoints[i-1].Position - waypoints[i-2].Position
			cur := waypoints[i].Position - waypoints[i-1].Position
			if prev != 0 && cur != 0 && (prev > 0) != (cur > 0) {
				reversals++
			}
		}
	}
	avg := sum / float64(len(waypoints)-1)
	var variance float64
	for i := 1; i < len(waypoints); i++ {
		d := float64(waypoints[i].TimeDelta)
		diff := d - avg
		variance += diff * diff
	}
	stddev := math.Sqrt(variance / float64(len(waypoints)-1))
	return map[string]any{
		"point_count":    len(waypoints),
		"delta_avg":      int(math.Round(avg)),
		"delta_stddev":   int(math.Round(stddev)),
		"delta_min":      int(minDelta),
		"delta_max":      int(maxDelta),
		"turn_reversals": reversals,
	}
}
