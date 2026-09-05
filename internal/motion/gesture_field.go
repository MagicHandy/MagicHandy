package motion

import "math"

type gestureBand struct {
	low, high, pace float64
}

// Reach and clock are correlated fields, not a choice between local/full
// patterns. Their interpolation is periodic for exact replay, while independent
// seeded scales avoid a fixed block cadence. End-only and full-only requests
// are hard spatial constraints; an intermediate mix expresses an attraction.
func gestureBands(s FlowSpec) []gestureBand {
	cycles := s.LoopCycles
	if cycles == 0 {
		cycles = 64
	}
	s.LoopCycles, s.MemoryCycles = cycles, max(2, s.MemoryCycles/2)
	g := s.Gesture
	full := float64(s.MaxPercent - s.MinPercent)
	anchors := gestureAnchors(s, cycles)
	mix, variation := float64(g.FocusMixPercent)/100, float64(g.VariationPercent)/100
	locality := make([]float64, cycles)
	impulse := make([]float64, cycles)
	for i := range cycles {
		u := float64(i)
		locality[i] = clampFloat(mix+4*math.Min(mix, 1-mix)*(s.driftField(u, 0x96185)-0.5), 0, 1)
		impulse[i] = s.driftField(u, 0x75ea1)
	}
	bands := make([]gestureBand, cycles)
	scale, remaining := 1.0, 0
	// Warm the bounded decay memory before retaining a single circular phrase.
	// A rebound changes subsequent excursions; it never inserts another pattern.
	for step := range cycles * 3 {
		i := step % cycles
		width := math.Max(10, float64(g.FocusWidthPercent)*(1-0.25*variation*s.driftField(float64(i), 0x75319)))
		if remaining == 0 && g.ReboundCount > 0 && mix > 0 && impulse[i] > impulse[(i+cycles-1)%cycles] && impulse[i] >= impulse[(i+1)%cycles] {
			remaining = g.ReboundCount
		}
		if remaining > 0 {
			next := scale * float64(g.ReboundDecayPercent) / 100
			if width*next >= 10 {
				scale = next
				remaining--
			} else {
				remaining = 0
			}
		} else {
			scale += (1 - scale) * 0.45
		}
		span := full + (math.Max(10, width*scale)-full)*locality[i]
		low := float64(s.MinPercent) + (full-span)*anchors[i]
		bands[i] = gestureBand{low: low, high: low + span,
			pace: 1 + 0.4*variation*(2*s.driftField(float64(i), 0x46a32)-1)}
	}
	return bands
}

// Roaming changes the working location independently of width and pace.
// At full roam the old focus carries no directional preference. At zero roam
// saved anchored scores retain their geometry. There is no region itinerary.
func gestureAnchors(s FlowSpec, cycles int) []float64 {
	g := s.Gesture
	anchor, roam := float64(g.FocusPercent)/100, float64(g.FocusRoamPercent)/100
	raw := make([]float64, cycles)
	s.MemoryCycles = max(8, s.MemoryCycles*2)
	for i := range raw {
		raw[i] = anchor*(1-roam) + roam*s.driftField(float64(i), 0x31d59)
	}
	// A periodic Lipschitz projection keeps neighboring windows overlapping,
	// including the seam and the narrowest legal stroke. Both envelopes are
	// bounded and reflection-symmetric; neither favors an end or a direction.
	step := 5 / float64(s.MaxPercent-s.MinPercent)
	anchors := make([]float64, cycles)
	for i := range anchors {
		low, high := raw[i], raw[i]
		for j, value := range raw {
			distance := math.Abs(float64(i - j))
			allowance := step * math.Min(distance, float64(cycles)-distance)
			low = math.Min(low, value+allowance)
			high = math.Max(high, value-allowance)
		}
		anchors[i] = (low + high) / 2
	}
	return anchors
}
