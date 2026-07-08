package motion

import (
	"math"
	"math/rand"
)

const proceduralStreamLeadMillis = 300

// ResolveAtrasoMS returns the smoothing delay between stroke points.
func ResolveAtrasoMS(physics ChaoticPhysics) int {
	var base int
	if physics.AtrasoMS > 0 {
		base = clampInt(physics.AtrasoMS, 1, 500)
	} else {
		switch normalizeTipoBatida(physics.TipoBatida) {
		case "very_fast", "vibrate", "turbo":
			base = 1
		case "alto":
			base = 40
		case "moderado":
			base = 80
		case "leve", "simples":
			base = 120
		case "lento", "fluido":
			base = 160
		default:
			base = 160
		}
	}
	return scaleAtrasoByVelocity(base, physics.Velocidade)
}

func scaleAtrasoByVelocity(atrasoMS, velocidade int) int {
	if atrasoMS <= 1 {
		return atrasoMS
	}
	if velocidade <= 0 {
		velocidade = 50
	}
	velocidade = clampInt(velocidade, 1, 100)
	ref := velocityToBaseDeltaMillis(50)
	current := velocityToBaseDeltaMillis(velocidade)
	if ref <= 0 {
		ref = 1
	}
	return clampInt(int(math.Round(float64(atrasoMS)*float64(current)/float64(ref))), 1, 500)
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

// GenerateStrokeWaypoints builds zone-aware full strokes with curved timing.
func GenerateStrokeWaypoints(
	physics ChaoticPhysics,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	return generateStrokeWaypoints(physics, durationMS, hardwareSafetyLock, rng, -1)
}

// GenerateStrokeWaypointsFromPosition continues motion from a device position (0..100).
func GenerateStrokeWaypointsFromPosition(
	physics ChaoticPhysics,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	return generateStrokeWaypoints(physics, durationMS, hardwareSafetyLock, rng, continueFrom)
}

func generateStrokeWaypoints(
	physics ChaoticPhysics,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	if rng == nil {
		// #nosec G404 -- deterministic fallback for unit tests.
		rng = rand.New(rand.NewSource(1))
	}
	if durationMS <= 0 {
		durationMS = EstimateChatMotionDurationMS(physics)
	}

	region, ok := chaosRegionRange(physics.Regiao)
	if !ok {
		region = regionRange{Min: 0, Max: 100}
	}
	atraso := ResolveAtrasoMS(physics)

	var out []ChaosWaypoint
	lastPos := continueFrom
	if lastPos >= 0 {
		low, high := zoneStrokeBounds(region, strokeFullnessForTipo(physics.TipoBatida))
		if lastPos < low || lastPos > high {
			entry := low
			if lastPos > high {
				entry = high
			}
			bridge := bridgeWaypoints(lastPos, entry, physics, atraso, hardwareSafetyLock, rng)
			out = append(out, bridge...)
			lastPos = entry
		}
	}
	for waypointDurationMS(out) < durationMS {
		stroke, endPos := buildContinuousZoneStroke(
			region,
			physics,
			atraso,
			hardwareSafetyLock,
			rng,
			lastPos,
		)
		if len(stroke) == 0 {
			break
		}
		out = appendBridgedWaypoints(out, stroke, physics, atraso, hardwareSafetyLock, rng)
		lastPos = endPos
		if len(out) > 2048 {
			break
		}
	}
	if len(out) > 0 {
		MotionDebugLog("ORG", "stroke_waypoints.go:generateStrokeWaypoints", "organic motion stats", map[string]any{
			"regiao":      physics.Regiao,
			"tipo_batida": physics.TipoBatida,
			"stats":       organicMotionStats(out),
		})
	}
	return out
}

// GenerateProceduralStream fills a freestyle segment with mixed stroke phases.
func GenerateProceduralStream(
	physics ChaoticPhysics,
	durationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	return generateProceduralStream(physics, durationMillis, hardwareSafetyLock, rng, -1)
}

// GenerateProceduralStreamFromPosition continues freestyle motion from a device position.
func GenerateProceduralStreamFromPosition(
	physics ChaoticPhysics,
	durationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	return generateProceduralStream(physics, durationMillis, hardwareSafetyLock, rng, continueFrom)
}

func generateProceduralStream(
	physics ChaoticPhysics,
	durationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	if durationMillis <= 0 {
		return generateStrokeWaypoints(physics, 0, hardwareSafetyLock, rng, continueFrom)
	}

	var out []ChaosWaypoint
	lastPos := continueFrom
	firstPhase := true
	for ChaosWaypointsDurationMS(out, proceduralStreamLeadMillis) < int(durationMillis) {
		remaining := durationMillis - int64(ChaosWaypointsDurationMS(out, proceduralStreamLeadMillis))
		if remaining <= 0 {
			break
		}
		phaseMS := int64(2200 + rng.Int63n(2801))
		if phaseMS > remaining {
			phaseMS = remaining
		}
		if phaseMS < 800 {
			phaseMS = remaining
		}

		phasePhysics := adjustPhysicsForPhase(physics, pickStreamPhaseTipo(physics, rng))
		phasePhysics.Regiao = pickStreamPhaseRegiao(physics.Regiao, rng)

		phaseFrom := -1
		if firstPhase {
			phaseFrom = lastPos
			firstPhase = false
		} else if len(out) > 0 {
			phaseFrom = out[len(out)-1].Position
		}
		phase := generateStrokeWaypoints(phasePhysics, int(phaseMS), hardwareSafetyLock, rng, phaseFrom)
		out = appendBridgedWaypoints(out, phase, phasePhysics, ResolveAtrasoMS(phasePhysics), hardwareSafetyLock, rng)
		if len(out) > 0 {
			lastPos = out[len(out)-1].Position
		}
		if len(out) > 4096 {
			break
		}
	}
	return out
}

func buildContinuousZoneStroke(
	region regionRange,
	physics ChaoticPhysics,
	atrasoMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	lastPos int,
) ([]ChaosWaypoint, int) {
	tipo := normalizeTipoBatida(physics.TipoBatida)
	switch tipo {
	case "very_fast", "vibrate", "turbo":
		stroke := buildVibrateStroke(region, physics, atrasoMS, hardwareSafetyLock, rng)
		endPos := lastPos
		if len(stroke) > 0 {
			endPos = stroke[len(stroke)-1].Position
		}
		return stroke, endPos
	default:
		fullness := strokeFullnessForTipo(tipo)
		low, high := zoneStrokeBounds(region, fullness)
		start := lastPos
		if start < 0 {
			if rng.Float64() < 0.5 {
				start = low
			} else {
				start = high
			}
		}
		start = clampInt(start, low, high)
		opposite := high
		if start >= (low+high)/2 {
			opposite = low
		}

		depth := organicStrokeDepth(rng)
		partialOpposite := start + int(math.Round(float64(opposite-start)*depth))
		partialOpposite = clampInt(partialOpposite, low, high)

		pointsPerLegDown := curvePointCount(atrasoMS) + rng.Intn(2)
		pointsPerLegUp := curvePointCount(atrasoMS) + rng.Intn(2)
		if pointsPerLegUp < 2 {
			pointsPerLegUp = 2
		}

		legDown := organicCurvedPositions(start, partialOpposite, pointsPerLegDown, region, rng)
		returnPos := organicReturnPosition(start, low, high, rng)
		legUp := organicCurvedPositions(partialOpposite, returnPos, pointsPerLegUp, region, rng)

		var positions []int
		if rng.Float64() < 0.14 && absIntValue(partialOpposite-start) > 6 {
			positions = append(positions, legDown...)
			positions = append(positions, teaseWiggleAt(partialOpposite, region, rng)...)
			positions = append(positions, legUp[1:]...)
		} else {
			positions = append(legDown, legUp[1:]...)
		}
		positions = sanitizePositionSteps(positions, region)
		return positionsToTimedWaypoints(positions, physics, atrasoMS, hardwareSafetyLock, rng), returnPos
	}
}

func appendBridgedWaypoints(
	out []ChaosWaypoint,
	stroke []ChaosWaypoint,
	physics ChaoticPhysics,
	atrasoMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	if len(stroke) == 0 {
		return out
	}
	if len(out) > 0 && out[len(out)-1].Position != stroke[0].Position {
		bridge := bridgeWaypoints(out[len(out)-1].Position, stroke[0].Position, physics, atrasoMS, hardwareSafetyLock, rng)
		if len(bridge) > 1 {
			out = append(out, bridge[1:]...)
		}
	}
	return append(out, stroke...)
}

func bridgeWaypoints(from, to int, physics ChaoticPhysics, atrasoMS int, hardwareSafetyLock bool, rng *rand.Rand) []ChaosWaypoint {
	if from == to {
		return []ChaosWaypoint{{TimeDelta: jitteredTimeDelta(atrasoMS, 0, physics.Intensidade, physics, hardwareSafetyLock, rng), Position: from}}
	}
	count := curvePointCount(atrasoMS)
	if absIntValue(to-from) < 8 {
		count = 2
	}
	positions := organicCurvedPositions(from, to, count, bridgeRegionFor(from, to), rng)
	return positionsToTimedWaypoints(positions, physics, atrasoMS, hardwareSafetyLock, rng)
}

func buildVibrateStroke(
	region regionRange,
	physics ChaoticPhysics,
	atrasoMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	low, high := vibrateBounds(region)
	span := high - low
	if span < 2 {
		span = 2
	}
	cycles := 8 + rng.Intn(10)
	var out []ChaosWaypoint
	for i := 0; i < cycles; i++ {
		l := low + rng.Intn(maxInt(1, span/3))
		h := high - rng.Intn(maxInt(1, span/3))
		if l >= h {
			l, h = low, high
		}
		dt := jitteredTimeDelta(atrasoMS, float64(i)/float64(cycles), physics.Intensidade, physics, hardwareSafetyLock, rng)
		if i%2 == 0 {
			out = append(out,
				ChaosWaypoint{TimeDelta: dt, Position: l},
				ChaosWaypoint{TimeDelta: jitteredTimeDelta(atrasoMS, 0.5, physics.Intensidade, physics, hardwareSafetyLock, rng), Position: h},
			)
		} else {
			out = append(out,
				ChaosWaypoint{TimeDelta: dt, Position: h},
				ChaosWaypoint{TimeDelta: jitteredTimeDelta(atrasoMS, 0.5, physics.Intensidade, physics, hardwareSafetyLock, rng), Position: l},
			)
		}
	}
	return out
}

func vibrateBounds(region regionRange) (int, int) {
	span := region.Max - region.Min
	if span < 8 {
		return region.Min, region.Max
	}
	low := region.Min + span*2/5
	high := region.Max - span/8
	if low >= high {
		low = region.Min + span/4
		high = region.Max - span/4
	}
	return low, high
}

func strokeFullnessForTipo(tipo string) float64 {
	switch tipo {
	case "simples", "fluido", "lento":
		return 1.0
	case "leve":
		return 0.72
	case "moderado":
		return 0.88
	case "alto":
		return 0.95
	default:
		return 1.0
	}
}

func zoneStrokeBounds(region regionRange, fullness float64) (int, int) {
	span := float64(region.Max - region.Min)
	margin := (1.0 - fullness) * span / 2
	low := int(math.Round(float64(region.Min) + margin))
	high := int(math.Round(float64(region.Max) - margin))
	if low >= high {
		low = region.Min
		high = region.Max
	}
	return low, high
}

func curvePointCount(atrasoMS int) int {
	switch {
	case atrasoMS <= 5:
		return 2
	case atrasoMS <= 50:
		return 3
	case atrasoMS <= 100:
		return 5
	default:
		return 6
	}
}

func curvedPositions(from, to int, count int) []int {
	if count < 2 {
		count = 2
	}
	out := make([]int, count)
	for i := 0; i < count; i++ {
		t := float64(i) / float64(count-1)
		eased := easeInOutSine(t)
		out[i] = from + int(math.Round(float64(to-from)*eased))
	}
	return out
}

func easeInOutSine(t float64) float64 {
	return 0.5 - 0.5*math.Cos(math.Pi*t)
}

func positionsToTimedWaypoints(
	positions []int,
	physics ChaoticPhysics,
	atrasoMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	intensidade := physics.Intensidade
	if intensidade <= 0 {
		intensidade = 50
	}
	out := make([]ChaosWaypoint, len(positions))
	denom := len(positions) - 1
	if denom < 1 {
		denom = 1
	}
	for i, pos := range positions {
		progress := float64(i) / float64(denom)
		delta := jitteredTimeDelta(atrasoMS, progress, intensidade, physics, hardwareSafetyLock, rng)
		out[i] = ChaosWaypoint{
			TimeDelta: delta,
			Position:  pos,
		}
	}
	return out
}

func clampDelta(atrasoMS int, hardwareSafetyLock bool) int {
	delta := atrasoMS
	if delta < 1 {
		delta = 1
	}
	if hardwareSafetyLock && delta < 30 {
		delta = 30
	}
	return delta
}

func waypointDurationMS(waypoints []ChaosWaypoint) int {
	total := 0
	for i := 1; i < len(waypoints); i++ {
		total += waypoints[i].TimeDelta
	}
	return total
}

func normalizeTipoBatida(value string) string {
	switch value {
	case "simples", "leve", "moderado", "alto", "fluido", "lento", "very_fast", "vibrate", "turbo":
		return value
	default:
		return "fluido"
	}
}

func adjustPhysicsForPhase(physics ChaoticPhysics, tipo string) ChaoticPhysics {
	next := physics
	next.TipoBatida = tipo
	switch tipo {
	case "lento":
		if next.Velocidade > 35 {
			next.Velocidade = 20 + next.Velocidade/4
		}
		if next.AtrasoMS == 0 {
			next.AtrasoMS = 160
		}
	case "fluido":
		if next.AtrasoMS == 0 {
			next.AtrasoMS = 160
		}
	case "alto", "very_fast", "vibrate", "turbo":
		if next.Velocidade < 55 {
			next.Velocidade = 55
		}
	}
	return next
}

func pickStreamPhaseTipo(physics ChaoticPhysics, rng *rand.Rand) string {
	anchor := normalizeTipoBatida(physics.TipoBatida)
	switch anchor {
	case "alto", "very_fast", "vibrate", "turbo":
		return weightedPick(rng, []weightedChoice{
			{"fluido", 0.25},
			{"lento", 0.15},
			{"moderado", 0.30},
			{"alto", 0.20},
			{"very_fast", 0.10},
		})
	case "leve", "simples":
		return weightedPick(rng, []weightedChoice{
			{"fluido", 0.45},
			{"lento", 0.25},
			{"leve", 0.20},
			{"moderado", 0.10},
		})
	default:
		return weightedPick(rng, []weightedChoice{
			{"fluido", 0.40},
			{"lento", 0.20},
			{"leve", 0.20},
			{"moderado", 0.15},
			{"alto", 0.05},
		})
	}
}

func pickStreamPhaseRegiao(anchor string, rng *rand.Rand) string {
	switch anchor {
	case "cabeca":
		return weightedPick(rng, []weightedChoice{
			{"cabeca", 0.70},
			{"meio_cabeca", 0.30},
		})
	case "base":
		return weightedPick(rng, []weightedChoice{
			{"meio_base", 0.45},
			{"base", 0.35},
			{"meio", 0.20},
		})
	case "meio":
		return weightedPick(rng, []weightedChoice{
			{"meio_cabeca", 0.45},
			{"meio", 0.35},
			{"cabeca", 0.20},
		})
	case "full", "completo", "cabeca_base":
		return weightedPick(rng, []weightedChoice{
			{"cabeca", 0.35},
			{"meio_cabeca", 0.40},
			{"meio", 0.15},
			{"full", 0.10},
		})
	default:
		return weightedPick(rng, []weightedChoice{
			{"meio_cabeca", 0.35},
			{"cabeca", 0.30},
			{"meio", 0.25},
			{"meio_base", 0.10},
		})
	}
}

type weightedChoice struct {
	value  string
	weight float64
}

func weightedPick(rng *rand.Rand, choices []weightedChoice) string {
	total := 0.0
	for _, choice := range choices {
		total += choice.weight
	}
	roll := rng.Float64() * total
	for _, choice := range choices {
		roll -= choice.weight
		if roll <= 0 {
			return choice.value
		}
	}
	return choices[len(choices)-1].value
}

func absIntValue(v int) int {
	if v < 0 {
		return -v
	}
	return v
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

func sanitizePositionSteps(positions []int, region regionRange) []int {
	if len(positions) < 2 {
		return positions
	}
	out := make([]int, len(positions))
	out[0] = positions[0]
	for i := 1; i < len(positions); i++ {
		pos := positions[i]
		step := absIntValue(pos - out[i-1])
		if step > 0 && step < 3 {
			if pos >= out[i-1] {
				pos = clampInt(out[i-1]+3, region.Min, region.Max)
			} else {
				pos = clampInt(out[i-1]-3, region.Min, region.Max)
			}
		}
		out[i] = pos
	}
	return out
}

func bridgeRegionFor(from, to int) regionRange {
	low := minInt(from, to)
	high := maxInt(from, to)
	if low < 0 {
		low = 0
	}
	if high > 100 {
		high = 100
	}
	return regionRange{Min: low, Max: high}
}
