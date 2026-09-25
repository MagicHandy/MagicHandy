package motion

import "math"

// Phrasing is the per-stroke timing and reach texture that keeps a Creative v2
// phrase from reading as a metronome: stretches that ease off, short flurries
// of quicker strokes, landing points that differ a little from stroke to
// stroke, accents that come and go, and turns that occasionally linger. Every
// value is drawn from the score's seed, so a saved score replays exactly, and
// every amount scales with variation_percent: zero repeats each stroke exactly.
//
// None of this is a second motion loop or a runtime random source. It shapes
// the stroke destinations and durations that the shared timing fit, sanitizer
// and transport already own.
type gesturePhrase struct {
	// timeScale multiplies the stroke's travel time: above 1 is slower.
	timeScale float64
	// flurrySpan shortens a flurry stroke where local reach is allowed.
	flurrySpan float64
	// accent scales the requested direction contrast for this stroke.
	accent float64
	// inertia is this stroke's realized inertia, 0..1.
	inertia float64
	// lingerTop and lingerBottom rest at the upper and following lower turn.
	lingerTop, lingerBottom float64
}

const (
	// breathDepth is the most a phrase eases off, as extra stroke time per unit
	// of variation. Breath only ever slows: it gives pace a floor that does not
	// depend on the model, and it never pushes above the chosen speed.
	breathDepth = 0.6
	// breathMemoryCycles sets the breath's phrase length, about 16 strokes.
	breathMemoryCycles = 16
	// flurryChance is the chance per stroke and unit of variation that a short
	// flurry of two to four quicker strokes starts there.
	flurryChance = 0.06
	// lingerChance is the chance per turn and unit of variation that a turn
	// rests briefly before reversing.
	lingerChance = 0.05
	// lingerMinimumSeconds keeps a rest above the shared minimum reversal gap.
	lingerMinimumSeconds = 0.11
	lingerSpanSeconds    = 0.17
	// landingInset is the most a reversal lands inside its planned endpoint,
	// as a share of the stroke per unit of variation.
	landingInset = 0.2
)

func (s FlowSpec) gesturePhrase(i, cycles int, variation float64) gesturePhrase {
	g := s.Gesture
	u := float64(i)
	phrase := gesturePhrase{timeScale: s.breath(i, variation), flurrySpan: 1,
		accent:  clampFloat(1+1.6*variation*(2*s.driftField(u, 0x5c6f7)-1), 0, 1.8),
		inertia: clampFloat(float64(g.InertiaPercent)/100*(1+variation*(2*s.driftField(u, 0x6d708)-1)), 0, 1)}
	// Correlated drift plus a small independent jitter: the pace wanders over a
	// few strokes and no two neighboring strokes take exactly the same time.
	phrase.timeScale *= 1 + 0.4*variation*(2*s.driftField(u, 0x46a32)-1)
	phrase.timeScale *= 1 + 0.15*variation*(2*flowHashUnit(s.Seed^0x3c6f, strokeKey(i))-1)
	if quick, span, ok := s.flurry(i, cycles, variation); ok {
		phrase.timeScale /= quick
		phrase.flurrySpan = span
	}
	phrase.lingerTop = s.linger(2*i, variation)
	phrase.lingerBottom = s.linger(2*i+1, variation)
	return phrase
}

// breath is a slow field that eases the pace over a phrase of about sixteen
// strokes and then recovers. It is smooth at both ends, so an easing stretch
// arrives and leaves gradually rather than as a step.
func (s FlowSpec) breath(i int, variation float64) float64 {
	slow := s
	slow.MemoryCycles = breathMemoryCycles
	depth := (0.6 - slow.driftField(float64(i), 0x9b3a1)) / 0.35
	depth = clampFloat(depth, 0, 1)
	depth = depth * depth * (3 - 2*depth)
	return 1 + breathDepth*variation*depth
}

// flurry reports whether stroke i falls inside a seeded flurry of two to four
// quicker strokes, and how much quicker and shorter they are.
func (s FlowSpec) flurry(i, cycles int, variation float64) (quick, span float64, ok bool) {
	for back := range 4 {
		start := strokeKey((i - back + cycles) % cycles)
		if flowHashUnit(s.Seed^0x4e1f, start) >= flurryChance*variation {
			continue
		}
		if back < 2+int(flowHashUnit(s.Seed^0x5f20, start)*3) {
			return 1.2 + 0.2*flowHashUnit(s.Seed^0x6031, start), 0.6, true
		}
	}
	return 1, 1, false
}

// linger returns a brief rest, in seconds, for a seeded minority of turns.
func (s FlowSpec) linger(turn int, variation float64) float64 {
	if flowHashUnit(s.Seed^0x7142, strokeKey(turn)) >= lingerChance*variation {
		return 0
	}
	return lingerMinimumSeconds + lingerSpanSeconds*flowHashUnit(s.Seed^0x8253, strokeKey(turn))
}

// landing moves a stroke's endpoints a little inside its planned band, drawn
// per stroke, so repeated strokes do not strike one identical point. The band
// still bounds every position, and a stroke never shrinks below ten points. An
// explicitly held end (local work anchored there with no roaming) is where the
// request asked the stroke to arrive, so it keeps landing exactly.
func (s FlowSpec) landing(i int, low, high, variation float64, holdLow, holdHigh bool) (float64, float64) {
	span := high - low
	inset := landingInset * variation * span
	if inset <= 0 || span-2*inset < 10 {
		return low, high
	}
	if !holdLow {
		low += inset * flowHashUnit(s.Seed^0x1a4d, strokeKey(i))
	}
	if !holdHigh {
		high -= inset * flowHashUnit(s.Seed^0x2b5e, strokeKey(i))
	}
	return low, high
}

// shortenForFlurry keeps the stroke's relative placement: an anchored local
// stroke stays at its anchor while it shortens.
func shortenForFlurry(low, span, flurrySpan, anchor float64) (float64, float64) {
	if flurrySpan >= 1 {
		return low, span
	}
	shortened := math.Max(10, span*flurrySpan)
	return low + (span-shortened)*anchor, shortened
}

// strokeKey keys a per-stroke or per-turn hash. Validated scores have at most
// 64 cycles, so indices stay far below the uint32 range.
func strokeKey(index int) uint32 {
	return uint32(max(0, index)) // #nosec G115 -- stroke and turn indices are bounded by the 64-cycle phrase.
}
