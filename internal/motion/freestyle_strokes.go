package motion

import "math"

// The Freestyle stream is generated one stroke at a time from smooth seeded
// fields over the stroke index. Pace, length and location each follow their
// own slow drift plus a quicker wobble, so the motion keeps evolving without a
// block of identical strokes or a switch to a different pattern. Sparse events
// (a flurry of quicker strokes, an easing stretch, a deep stroke, teasing at
// the tip) ease in and out over neighboring strokes. Every value depends only
// on the seed, the stroke index and the keyframed controls there: there is no
// state carried from stroke to stroke, which is what lets overlapping windows
// agree exactly.

const (
	// freestyleContextStrokes surround a window when its strokes are timed. The
	// joint timing fit settles within seventeen legs, so a window's own strokes
	// are timed exactly as in any other window that contains them.
	freestyleContextStrokes = 12
	// freestyleDurationExponent makes short strokes quicker in tempo but
	// gentler in travel speed than long ones, as a hand does.
	freestyleDurationExponent = 0.55
	// freestyleBlockStrokes is the nominal spacing of the event blocks.
	freestyleBlockStrokes = 14
	// freestyleWarmStrokes ease the stream in at its start and out at its end.
	freestyleWarmStrokes   = 6
	freestyleMinimumStroke = 10.0
	// freestyleRestSeconds is the length of one resting context leg.
	freestyleRestSeconds = 0.4
)

// Salts for the independent seeded fields.
const (
	saltTempoSlow uint32 = 0x2f6b1d3 + iota*0x9e37
	saltTempoMid
	saltLengthSlow
	saltLengthMid
	saltPlace
	saltPlaceMid
	saltWobble
	saltAccent
	saltAccentDrift
	saltInertia
	saltJitter
	saltLingerTop
	saltLingerTopLength
	saltLingerBottom
	saltLingerBottomLength
	saltLandLow
	saltLandHigh
	saltBlock
	saltEventPick
	saltEventCount
	saltEventOffset
	saltEventDepth
	saltEventShape
)

type freestyleEventKind uint8

const (
	eventNone freestyleEventKind = iota
	eventFlurry
	eventEase
	eventDeep
	eventTease
)

// freestyleEvent is one stroke's share of an event: amount is its eased
// envelope times the event's depth, and pick shapes its target reach.
type freestyleEvent struct {
	kind   freestyleEventKind
	amount float64
	pick   float64
}

type freestyleStroke struct {
	low, high float64
	// rate is the full-stroke travel rate, in percent per second, at this
	// stroke's pace.
	rate                 float64
	upWeight, downWeight float64
	inertia, softness    float64
	// restTop and restBottom are brief rests, in seconds, at this stroke's
	// upper turn and at the lower turn that follows it.
	restTop, restBottom float64
	resting             bool
}

type freestyleStream struct {
	spec               FreestyleSpec
	seed               uint32
	minSpeed, maxSpeed float64
	handyModel         string
	track              keyframeTrack
}

func newFreestyleStream(spec FlowSpec, handyModel string) freestyleStream {
	f := *spec.Freestyle
	return freestyleStream{spec: f, seed: spec.Seed, minSpeed: float64(f.MinSpeedPercent),
		maxSpeed: float64(spec.SpeedPercent), handyModel: handyModel, track: newKeyframeTrack(f.Keyframes)}
}

func (f freestyleStream) hash(salt uint32, index int) float64 {
	return flowHashUnit(f.seed^salt, uint32(index)) // #nosec G115 -- intentional two's-complement wrap; the value only keys a hash.
}

// noise is non-periodic value noise along the stroke index: seeded lattice
// values every memory strokes, joined by the C3 septic interpolant. Memory is
// fixed per field; a changing memory would scroll the lattice.
func (f freestyleStream) noise(salt uint32, k int, memory float64) float64 {
	x := float64(k)/memory + flowHashUnit(f.seed^salt, 0x51ed)
	cell := math.Floor(x)
	index := int(cell)
	left, right := f.hash(salt, index), f.hash(salt, index+1)
	return left + (right-left)*flowSeptic(x-cell)
}

// signed maps a noise field to -1..1.
func (f freestyleStream) signed(salt uint32, k int, memory float64) float64 {
	return 2*f.noise(salt, k, memory) - 1
}

func (f freestyleStream) resting(k int) bool {
	return k < f.spec.FromStroke || (f.spec.EndStroke > 0 && k >= f.spec.EndStroke)
}

// warmth eases the first strokes in and the final strokes out.
func (f freestyleStream) warmth(k int) float64 {
	warm := freestyleSmooth(float64(k-f.spec.FromStroke+1) / freestyleWarmStrokes)
	if f.spec.EndStroke > 0 {
		warm = math.Min(warm, freestyleSmooth(float64(f.spec.EndStroke-k)/freestyleWarmStrokes))
	}
	return warm
}

func freestyleSmooth(x float64) float64 {
	x = clampFloat(x, 0, 1)
	return x * x * (3 - 2*x)
}

// stroke generates stroke k before its lower turn is reconciled with the
// stroke before it.
func (f freestyleStream) stroke(k int) freestyleStroke {
	if f.resting(k) {
		return freestyleStroke{resting: true}
	}
	c := f.track.at(k)
	event := f.eventAt(k)
	warmth := f.warmth(k)
	intensity := f.intensity(k, c, event, warmth)
	length, anchor := f.reach(k, c, event, warmth)
	low := (100 - length) * anchor
	high := low + length
	if inset := 0.15 * c.variety * length; inset > 0 && length-2*inset >= freestyleMinimumStroke &&
		(event.kind != eventDeep || event.amount < 0.5) {
		low += inset * f.hash(saltLandLow, k)
		high -= inset * f.hash(saltLandHigh, k)
	}
	stroke := freestyleStroke{low: low, high: high,
		rate: rateForFractionalSpeed(f.minSpeed+(f.maxSpeed-f.minSpeed)*intensity, f.handyModel)}
	stroke.upWeight, stroke.downWeight, stroke.inertia, stroke.softness = f.texture(k, c, intensity, event)
	stroke.restTop, stroke.restBottom = f.lingers(k, c, event)
	return stroke
}

// intensity places the stroke's pace inside the speed band: the chosen pace,
// scaled by the session shape's energy, drifting slowly with variety.
func (f freestyleStream) intensity(k int, c streamControls, event freestyleEvent, warmth float64) float64 {
	floor := math.Min(c.pace, 0.05)
	value := floor + (c.pace-floor)*c.energy
	value += c.variety * (0.36*f.signed(saltTempoSlow, k, 26) + 0.16*f.signed(saltTempoMid, k, 6))
	switch event.kind {
	case eventFlurry:
		value += 0.3 * event.amount
	case eventEase:
		value -= 0.3 * event.amount
	case eventDeep:
		value -= 0.12 * event.amount
	case eventTease:
		value -= 0.06 * event.amount
	}
	value *= 0.35 + 0.65*warmth
	return clampFloat(value, 0, 1)
}

// reach returns the stroke's length and where it sits: anchor 0 places the
// stroke at the base, 1 at the tip.
func (f freestyleStream) reach(k int, c streamControls, event freestyleEvent, warmth float64) (length, anchor float64) {
	v := c.variety
	length = (12 + 88*c.length) * math.Exp(v*(0.55*f.signed(saltLengthSlow, k, 18)+0.2*f.signed(saltLengthMid, k, 5)))
	// The place field is stretched a little so full roaming visits the ends
	// of the range rather than hovering around the middle.
	field := clampFloat(0.5+1.3*(0.75*f.noise(saltPlace, k, 24)+0.25*f.noise(saltPlaceMid, k, 7)-0.5), 0, 1)
	anchor = c.focus*(1-c.roaming) + c.roaming*field + 0.06*v*f.signed(saltWobble, k, 5)
	anchor += (0.9 - anchor) * 0.7 * c.tease
	length += (math.Min(length, 24) - length) * 0.6 * c.tease
	switch event.kind {
	case eventFlurry:
		length *= 1 - 0.4*event.amount
	case eventEase:
		length *= 1 + 0.1*event.amount
	case eventDeep:
		length += (math.Max(length, 88+12*event.pick) - length) * event.amount
		anchor += (0.5 - anchor) * event.amount
	case eventTease:
		length += (math.Min(length, 13+9*event.pick) - length) * event.amount
		anchor += (1 - anchor) * event.amount
	}
	length *= 0.75 + 0.25*warmth
	return clampFloat(length, freestyleMinimumStroke, 100), clampFloat(anchor, 0, 1)
}

// texture returns the stroke's direction contrast as leg time weights, its
// velocity crest and how softly it turns. Slow and teasing strokes turn more
// softly; quick strokes keep a rounded, continuous turn.
func (f freestyleStream) texture(k int, c streamControls, intensity float64, event freestyleEvent) (up, down, inertia, softness float64) {
	v := c.variety
	contrast := 0.38*c.accent*(1+0.5*v*f.signed(saltAccent, k, 12)) + 0.08*v*f.signed(saltAccentDrift, k, 20)
	contrast = clampFloat(contrast, -0.6, 0.6)
	jitter := 1 + 0.1*v*(2*f.hash(saltJitter, k)-1)
	up, down = (1-0.65*contrast)*jitter, (1+0.65*contrast)*jitter
	inertia = clampFloat(0.28+0.22*v*f.signed(saltInertia, k, 9), 0, 0.9)
	softness = 0.06 + 0.3*(1-intensity)*(1-intensity)
	switch event.kind {
	case eventEase:
		softness += 0.3 * event.amount
	case eventTease:
		softness += 0.15 * event.amount
	}
	return up, down, inertia, clampFloat(softness, 0, 0.7)
}

// lingers are brief rests at a seeded minority of turns. Teasing lingers at
// the tip; a flurry never pauses.
func (f freestyleStream) lingers(k int, c streamControls, event freestyleEvent) (top, bottom float64) {
	if event.kind == eventFlurry && event.amount > 0 {
		return 0, 0
	}
	topChance, bottomChance := 0.03*c.variety, 0.03*c.variety
	if event.kind == eventTease {
		topChance += 0.2 * event.amount
	}
	if f.hash(saltLingerTop, k) < topChance {
		top = 0.12 + 0.22*f.hash(saltLingerTopLength, k)
	}
	if f.hash(saltLingerBottom, k) < bottomChance {
		bottom = 0.12 + 0.18*f.hash(saltLingerBottomLength, k)
	}
	return top, bottom
}

// blockStart is the first stroke of event block b. The grid is jittered so
// events keep no fixed spacing; consecutive blocks are 8-20 strokes long.
func (f freestyleStream) blockStart(b int) int {
	return b*freestyleBlockStrokes + int(f.hash(saltBlock, b)*7) - 3
}

func (f freestyleStream) blockOf(k int) int {
	b := int(math.Floor(float64(k) / freestyleBlockStrokes))
	for f.blockStart(b) > k {
		b--
	}
	for f.blockStart(b+1) <= k {
		b++
	}
	return b
}

// eventAt returns stroke k's share of its block's event, if any. A block
// holds at most one event, drawn from the controls at the block's start.
func (f freestyleStream) eventAt(k int) freestyleEvent {
	block := f.blockOf(k)
	start := f.blockStart(block)
	span := f.blockStart(block+1) - start
	kind, count := f.eventKind(block, f.track.at(start))
	if kind == eventNone {
		return freestyleEvent{}
	}
	count = min(count, span)
	first := start + int(f.hash(saltEventOffset, block)*float64(span-count+1))
	if k < first || k >= first+count {
		return freestyleEvent{}
	}
	ramp := 1.0
	switch {
	case kind == eventDeep:
		ramp = 0
	case count > 6:
		ramp = 2
	}
	j := float64(k - first)
	envelope := freestyleSmooth((j+1)/(ramp+1)) * freestyleSmooth((float64(count)-j)/(ramp+1))
	depth := 0.6 + 0.4*f.hash(saltEventDepth, block)
	return freestyleEvent{kind: kind, amount: envelope * depth, pick: f.hash(saltEventShape, block)}
}

// eventKind draws a block's event. Variety makes every event more likely;
// teasing makes teasing, and the occasional deep stroke that answers it, the
// usual event and rules out flurries.
func (f freestyleStream) eventKind(block int, c streamControls) (freestyleEventKind, int) {
	v, t := c.variety, c.tease
	weights := [...]float64{
		eventFlurry: 0.28 * v * (1 - 0.8*t),
		eventEase:   0.2 * v,
		eventDeep:   0.24*v*(1-0.7*c.length) + 0.15*t,
		eventTease:  0.16*v + 0.85*t,
	}
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	chance := math.Min(0.9, total)
	pick := f.hash(saltEventPick, block)
	if total <= 0 || pick >= chance {
		return eventNone, 0
	}
	pick *= total / chance
	size := f.hash(saltEventCount, block)
	for kind, weight := range weights {
		if kind == int(eventNone) {
			continue
		}
		if pick < weight {
			switch freestyleEventKind(kind) {
			case eventFlurry:
				return eventFlurry, 3 + int(size*3)
			case eventEase:
				return eventEase, 5 + int(size*5)
			case eventDeep:
				return eventDeep, 1 + int(size*2.5)
			default:
				return eventTease, 4 + int(size*(4+6*t))
			}
		}
		pick -= weight
	}
	return eventNone, 0
}

// rateForFractionalSpeed interpolates the calibrated travel rate between whole
// speed percentages, so a gradual pace change has no steps.
func rateForFractionalSpeed(speed float64, handyModel string) float64 {
	speed = clampFloat(speed, 1, 100)
	lower := int(math.Floor(speed))
	rate := referenceTravelRateForSpeed(lower, handyModel)
	if lower >= 100 {
		return rate
	}
	return rate + (referenceTravelRateForSpeed(lower+1, handyModel)-rate)*(speed-float64(lower))
}
