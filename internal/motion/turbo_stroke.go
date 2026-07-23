package motion

import (
	"math/rand"
)

	const (
	turboVibrateRangeDefault   = 18
	turboVibrateRangeMin       = 10
	turboVibrateRangeMax       = 35
	turboVibrateHalfCycleMinMS = 28
	turboVibrateHalfCycleMaxMS = 55
	veryFastStepMinMS          = 18
	veryFastStepMaxMS          = 38
)

// IsTurboTipo reports stroke types routed through turbo_stroke (very_fast sweep or buzz).
func IsTurboTipo(tipo string) bool {
	switch normalizeTipoBatida(tipo) {
	case "very_fast", "vibrate", "turbo":
		return true
	default:
		return false
	}
}

// IsTurboBuzzTipo reports short-range buzz modes (vibrate, turbo).
func IsTurboBuzzTipo(tipo string) bool {
	switch normalizeTipoBatida(tipo) {
	case "vibrate", "turbo":
		return true
	default:
		return false
	}
}

// GenerateTurboWaypointsForDuration fills durationMS with rapid low↔high oscillation.
func GenerateTurboWaypointsForDuration(
	physics ChaoticPhysics,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	if rng == nil {
		rng = rand.New(rand.NewSource(1)) // #nosec G404
	}
	if durationMS <= 0 {
		durationMS = EstimateChatMotionDurationMS(physics)
	}
	hardwareSafetyLock = false

	region := effectiveTurboRegion(physics)
	tipo := normalizeTipoBatida(physics.TipoBatida)
	halfCycleMS := turboVibrateHalfCycleMS(physics.Velocidade, tipo)

	center := turboOscillationCenter(region, continueFrom, tipo)
	low, high := turboOscillationBounds(region, physics, center)
	if high-low < turboVibrateRangeMin {
		low, high = expandTurboRange(center, high-low, turboVibrateRangeMin, region.Min, region.Max)
	}

	var out []ChaosWaypoint
	if continueFrom >= 0 && (continueFrom < low || continueFrom > high) {
		entry := clampInt(center, low, high)
		out = append(out, bridgeTurboWaypoints(continueFrom, entry, halfCycleMS, false)...)
	}

	var stream []ChaosWaypoint
	switch tipo {
	case "vibrate", "turbo":
		stream = generateTurboHardwareVibrate(low, high, halfCycleMS, durationMS)
	case "very_fast":
		stream = generateVeryFastSweep(region, physics, durationMS, rng)
	default:
		stream = generateTurboHardwareVibrate(low, high, halfCycleMS, durationMS)
	}
	out = append(out, stream...)

	if len(out) > 0 {
		MotionDebugLog("TRB", "turbo_stroke.go:GenerateTurboWaypointsForDuration", "turbo stats", map[string]any{
			"regiao":      physics.Regiao,
			"tipo_batida": physics.TipoBatida,
			"bounds":      []int{low, high},
			"span":        high - low,
			"half_cycle":  halfCycleMS,
			"stats":       organicMotionStats(out),
		})
	}
	return out
}

// generateVeryFastSweep runs rapid full-zone strokes (distinct from turbo buzz).
func generateVeryFastSweep(region regionRange, physics ChaoticPhysics, durationMS int, rng *rand.Rand) []ChaosWaypoint {
	low, high := region.Min, region.Max
	span := high - low
	if span < 4 {
		span = 4
	}
	stepMS := veryFastStepMS(physics.Velocidade)
	stepPos := clampInt(span*physics.Velocidade/140, 6, span)
	if stepPos < 4 {
		stepPos = 4
	}

	pos := low + span/2
	if rng != nil && rng.Float64() < 0.5 {
		pos = high
	}
	direction := 1
	if pos >= high {
		direction = -1
	}

	out := make([]ChaosWaypoint, 0, durationMS/stepMS+8)
	elapsed := 0
	for elapsed < durationMS && len(out) < 2048 {
		next := pos + direction*stepPos
		if next >= high {
			next = high
			direction = -1
		} else if next <= low {
			next = low
			direction = 1
		}
		out = append(out, ChaosWaypoint{TimeDelta: stepMS, Position: next})
		pos = next
		elapsed += stepMS
	}
	return out
}

func veryFastStepMS(velocidade int) int {
	velocidade = clampInt(velocidade, 1, 100)
	return clampInt(veryFastStepMaxMS-velocidade/4, veryFastStepMinMS, veryFastStepMaxMS)
}

// effectiveTurboRegion returns semantic zone bounds for turbo/very_fast generation.
func effectiveTurboRegion(physics ChaoticPhysics) regionRange {
	return effectiveSemanticRegion(physics)
}

// generateTurboHardwareVibrate alternates strictly between low and high endpoints.
func generateTurboHardwareVibrate(low, high, halfCycleMS, durationMS int) []ChaosWaypoint {
	if low > high {
		low, high = high, low
	}
	if high-low < turboVibrateRangeMin {
		center := (low + high) / 2
		low, high = expandTurboRange(center, high-low, turboVibrateRangeMin, 0, 100)
	}
	if halfCycleMS < turboVibrateHalfCycleMinMS {
		halfCycleMS = turboVibrateHalfCycleMinMS
	}

	out := make([]ChaosWaypoint, 0, durationMS/halfCycleMS+4)
	elapsed := 0
	target := low
	for elapsed < durationMS && len(out) < 2048 {
		out = append(out, ChaosWaypoint{TimeDelta: halfCycleMS, Position: target})
		elapsed += halfCycleMS
		if target == low {
			target = high
		} else {
			target = low
		}
	}
	return out
}

func expandTurboRange(center, currentSpan, wantSpan, minBound, maxBound int) (int, int) {
	if wantSpan < turboVibrateRangeMin {
		wantSpan = turboVibrateRangeMin
	}
	if currentSpan >= wantSpan {
		half := currentSpan / 2
		return center - half, center + half
	}
	half := wantSpan / 2
	low := center - half
	high := low + wantSpan
	if low < minBound {
		shift := minBound - low
		low += shift
		high += shift
	}
	if high > maxBound {
		shift := high - maxBound
		low -= shift
		high -= shift
	}
	if low < minBound {
		low = minBound
	}
	if high > maxBound {
		high = maxBound
	}
	if high-low < turboVibrateRangeMin && maxBound-minBound >= turboVibrateRangeMin {
		low = minBound
		high = minBound + turboVibrateRangeMin
	}
	return low, high
}

func turboVibrateHalfCycleMS(velocidade int, tipo string) int {
	velocidade = clampInt(velocidade, 1, 100)
	base := turboVibrateHalfCycleMaxMS - velocidade/3
	if normalizeTipoBatida(tipo) == "turbo" {
		base += 4
	}
	return clampInt(base, turboVibrateHalfCycleMinMS, turboVibrateHalfCycleMaxMS)
}

func turboOscillationCenter(region regionRange, continueFrom int, tipo string) int {
	_ = tipo
	return region.Min + (region.Max-region.Min)/2
}

func turboOscillationBounds(region regionRange, physics ChaoticPhysics, center int) (int, int) {
	tipo := normalizeTipoBatida(physics.TipoBatida)
	if tipo == "very_fast" {
		return region.Min, region.Max
	}
	if tipo == "vibrate" {
		return vibrateFullZoneBounds(region)
	}
	rangePts := turboVibrateRangeDefault
	if tipo == "turbo" {
		rangePts = clampInt(16+physics.Intensidade/12, turboVibrateRangeMin+2, turboVibrateRangeMax)
	}
	return turboShortRangeBounds(region, center, rangePts)
}

// vibrateFullZoneBounds uses the full semantic zone (min 10pt travel).
func vibrateFullZoneBounds(region regionRange) (int, int) {
	low, high := region.Min, region.Max
	span := high - low
	if span >= turboVibrateRangeMin {
		return low, high
	}
	center := low + span/2
	return expandTurboRange(center, span, turboVibrateRangeMin, region.Min, region.Max)
}

func turboShortRangeBounds(region regionRange, center, rangePts int) (int, int) {
	if rangePts < turboVibrateRangeMin {
		rangePts = turboVibrateRangeMin
	}
	span := region.Max - region.Min
	if span <= rangePts {
		return region.Min, region.Max
	}
	if center < region.Min || center > region.Max {
		center = region.Min + span/2
	}
	low := center - rangePts/2
	high := low + rangePts
	if low < region.Min {
		shift := region.Min - low
		low += shift
		high += shift
	}
	if high > region.Max {
		shift := high - region.Max
		low -= shift
		high -= shift
	}
	if low < region.Min {
		low = region.Min
	}
	if high > region.Max {
		high = region.Max
	}
	return low, high
}

func bridgeTurboWaypoints(from, to, deltaMS int, hardwareSafetyLock bool) []ChaosWaypoint {
	if from == to {
		return []ChaosWaypoint{{TimeDelta: turboPointDeltaMS(deltaMS, hardwareSafetyLock), Position: from}}
	}
	count := 2
	if absIntValue(to-from) > 20 {
		count = 3
	}
	positions := curvedPositions(from, to, count)
	out := make([]ChaosWaypoint, len(positions))
	dt := turboPointDeltaMS(deltaMS, hardwareSafetyLock)
	for i, pos := range positions {
		out[i] = ChaosWaypoint{TimeDelta: dt, Position: pos}
	}
	return out
}

func turboPointDeltaMS(atrasoMS int, hardwareSafetyLock bool) int {
	if atrasoMS <= 1 {
		return 1
	}
	return clampDelta(atrasoMS, hardwareSafetyLock)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// TurboWaypointSpan returns min/max positions in a turbo stream.
func TurboWaypointSpan(waypoints []ChaosWaypoint) (int, int) {
	if len(waypoints) == 0 {
		return 0, 0
	}
	minPos, maxPos := waypoints[0].Position, waypoints[0].Position
	for _, wp := range waypoints {
		if wp.Position < minPos {
			minPos = wp.Position
		}
		if wp.Position > maxPos {
			maxPos = wp.Position
		}
	}
	return minPos, maxPos
}
