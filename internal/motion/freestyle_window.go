package motion

import (
	"errors"
	"math"
	"sort"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// compileFreestyleCurve times the window's strokes together with the strokes
// around them and returns the window's own strokes as one finite curve, with
// the index that maps stream strokes to window time.
func compileFreestyleCurve(spec FlowSpec, handyModel string) (Curve, *streamWindow, error) {
	stream := newFreestyleStream(spec, handyModel)
	f := stream.spec
	first := f.StartStroke - freestyleContextStrokes
	strokes := stream.generate(first, f.StartStroke+f.Strokes+freestyleContextStrokes)
	budget := legBudget{velocity: referenceTravelRateForSpeed(100, handyModel),
		acceleration: runtimeMaxAccelerationPercentPerSecond2, jerk: runtimeMaxJerkPercentPerSecond3}
	legs, durations, strokeLeg := stream.legs(strokes, first, budget)
	curves, err := fitLegCurves(legs, durations, budget)
	if err != nil {
		return Curve{}, nil, errors.New("freestyle stroke timing did not converge inside motion limits")
	}
	from, to := strokeLeg[f.StartStroke-first], strokeLeg[f.StartStroke+f.Strokes-first]
	played := append([]Curve(nil), curves[from:to]...)
	for index, leg := range legs[from:to] {
		if leg.rest > 0 {
			played[index] = restCurve(leg.from, played[index].duration)
		}
	}
	curve, err := assembleLegs(legs[from:to], played, "freestyle", false)
	if err != nil {
		return Curve{}, nil, err
	}
	window := &streamWindow{seed: spec.Seed, startStroke: f.StartStroke, endStroke: f.EndStroke,
		strokeStarts: make([]int64, 0, f.Strokes+1)}
	elapsed := int64(0)
	next := from
	for stroke := range f.Strokes + 1 {
		for ; next < strokeLeg[f.StartStroke+stroke-first]; next++ {
			elapsed += curves[next].duration
		}
		window.strokeStarts = append(window.strokeStarts, elapsed)
	}
	return curve, window, nil
}

// restCurve is a rest as two points. The fitted rest has nine identical
// points; the shorter form keeps a lively window inside the point budget.
func restCurve(position float64, duration int64) Curve {
	points := []CurvePoint{{0, position}, {duration, position}}
	zero := []float64{0, 0}
	return Curve{points: points, slopes: zero, accelerations: zero,
		quintics: buildQuinticSegments(points, zero, zero), duration: duration}
}

// generate returns strokes first..last inclusive with each lower turn
// reconciled against the stroke before it: every descent is a genuine
// reversal of at least ten points. Lowering a turn only lengthens its stroke.
func (f freestyleStream) generate(first, last int) []freestyleStroke {
	strokes := make([]freestyleStroke, last-first+1)
	for index := range strokes {
		strokes[index] = f.stroke(first + index)
	}
	for index := 1; index < len(strokes); index++ {
		if !strokes[index].resting && !strokes[index-1].resting {
			strokes[index].low = math.Min(strokes[index].low, strokes[index-1].high-freestyleMinimumStroke)
		}
	}
	return strokes
}

// legs turns the generated strokes into alternating legs, with rests where a
// stroke lingers and resting legs before the stream starts or after it ends.
// strokeLeg holds each stroke's first leg and, last, the end of the final
// stroke; the final generated stroke only informs the descent before it.
func (f freestyleStream) legs(strokes []freestyleStroke, first int, budget legBudget) ([]gestureLeg, []int64, []int) {
	legs := make([]gestureLeg, 0, len(strokes)*2+8)
	rates := make([]float64, 0, cap(legs))
	strokeLeg := make([]int, 0, len(strokes))
	rest := func(position, seconds float64) {
		legs = append(legs, gestureLeg{from: position, to: position, rest: seconds})
		rates = append(rates, 0)
	}
	for index := range len(strokes) - 1 {
		strokeLeg = append(strokeLeg, len(legs))
		s, next := strokes[index], strokes[index+1]
		if s.resting {
			rest(f.restPosition(strokes, first, first+index), freestyleRestSeconds)
			continue
		}
		nextLow, nextRate, nextInertia := next.low, next.rate, next.inertia
		if next.resting {
			nextLow, nextRate, nextInertia = s.low, s.rate, s.inertia
		}
		legs = append(legs, gestureLeg{from: s.low, to: s.high, weight: s.upWeight, inertia: s.inertia, softness: s.softness})
		rates = append(rates, s.rate)
		if s.restTop > 0 {
			rest(s.high, s.restTop)
		}
		legs = append(legs, gestureLeg{from: s.high, to: nextLow, weight: s.downWeight,
			inertia: (s.inertia + nextInertia) / 2, softness: s.softness})
		rates = append(rates, math.Sqrt(s.rate*nextRate))
		if s.restBottom > 0 {
			rest(nextLow, s.restBottom)
		}
	}
	strokeLeg = append(strokeLeg, len(legs))
	durations := make([]int64, len(legs))
	for index, leg := range legs {
		if leg.rest > 0 {
			durations[index] = int64(math.Ceil(leg.rest * 1000))
			continue
		}
		seconds := 100 / rates[index] * math.Pow(math.Abs(leg.to-leg.from)/100, freestyleDurationExponent) * leg.weight
		floor := legTimeScale(gestureLegCurve(leg, 1000), budget)
		durations[index] = int64(math.Ceil(math.Max(seconds, floor) * 1000 * 1.002))
	}
	return legs, durations, strokeLeg
}

// restPosition is where a resting stroke waits: at the first stroke's lower
// turn before the stream starts, and back at the last stroke's lower turn
// after it ends.
func (f freestyleStream) restPosition(strokes []freestyleStroke, first, k int) float64 {
	if k < f.spec.FromStroke {
		return strokes[f.spec.FromStroke-first].low
	}
	return strokes[f.spec.EndStroke-1-first].low
}

// FreestyleProgress locates the playhead in the running Freestyle window. It
// is backend planning state for the mode manager, never sent to clients.
type FreestyleProgress struct {
	Seed        uint32 `json:"-"`
	StartStroke int    `json:"-"`
	// EndStroke is one past the window's last stroke.
	EndStroke int `json:"-"`
	// PlayingStroke is the stroke the device is estimated to be playing.
	PlayingStroke int `json:"-"`
	// EditableStroke is the first stroke that starts far enough beyond the
	// queued handoff that a new ramp there cannot touch queued motion.
	EditableStroke int `json:"-"`
	// RemainingMillis is the window time left after the playhead.
	RemainingMillis int64 `json:"-"`
	// StrokeMillis is the window's mean stroke duration.
	StrokeMillis int64 `json:"-"`
}

// freestyleEditLeadMillis keeps a control ramp clear of the queued handoff
// and the bounded retarget transition after it.
const freestyleEditLeadMillis = 1500

func (p MotionPlan) freestyleProgress(playbackMillis, handoffMillis int64) *FreestyleProgress {
	window := p.streamWindow()
	if window == nil || p.curve.duration <= 0 {
		return nil
	}
	starts := window.strokeStarts
	strokes := len(starts) - 1
	playing := p.PhaseAt(playbackMillis) * float64(p.curve.duration)
	handoff := p.PhaseAt(handoffMillis) * float64(p.curve.duration)
	editable := sort.Search(strokes, func(i int) bool { return float64(starts[i]) >= handoff+freestyleEditLeadMillis })
	return &FreestyleProgress{Seed: window.seed, StartStroke: window.startStroke,
		EndStroke: window.startStroke + strokes, PlayingStroke: window.startStroke + window.strokeAt(playing),
		EditableStroke:  window.startStroke + editable,
		RemainingMillis: int64(math.Max(0, float64(p.curve.duration)-playing)),
		StrokeMillis:    p.curve.duration / int64(strokes)}
}

func (p MotionPlan) streamWindow() *streamWindow {
	if p.Target.prepared == nil || p.Loop {
		return nil
	}
	return p.Target.prepared.stream
}

// strokeAt is the index of the played stroke containing window time at.
func (w streamWindow) strokeAt(at float64) int {
	strokes := len(w.strokeStarts) - 1
	index := sort.Search(strokes, func(i int) bool { return float64(w.strokeStarts[i]) > at }) - 1
	return clamp(index, 0, strokes-1)
}

// freestyleContinuation maps the playhead onto the same stroke of a new
// window of the same stream. Overlapping windows share their strokes exactly,
// so the shared transition then finds nothing to blend. A stroke the new
// window does not contain, or a different stream, falls back to the ordinary
// nearest-phase handoff.
func freestyleContinuation(previous MotionPlan, target MotionTarget, at int64) (float64, bool) {
	old := previous.streamWindow()
	next := target.prepared
	if old == nil || next == nil || next.stream == nil || old.seed != next.stream.seed {
		return 0, false
	}
	oldTime := previous.PhaseAt(at) * float64(previous.curve.duration)
	index := old.strokeAt(oldTime)
	stroke := old.startStroke + index
	newIndex := stroke - next.stream.startStroke
	if newIndex < 0 || newIndex >= len(next.stream.strokeStarts)-1 || next.curve.duration <= 0 {
		return 0, false
	}
	offset := oldTime - float64(old.strokeStarts[index])
	oldLength := old.strokeStarts[index+1] - old.strokeStarts[index]
	newLength := next.stream.strokeStarts[newIndex+1] - next.stream.strokeStarts[newIndex]
	if oldLength != newLength && oldLength > 0 {
		offset *= float64(newLength) / float64(oldLength)
	}
	return (float64(next.stream.strokeStarts[newIndex]) + offset) / float64(next.curve.duration), true
}

// chooseFreestylePhase compiles a Freestyle window once and, when the running
// content is the same stream, continues the stroke that is playing.
func chooseFreestylePhase(previous MotionPlan, target MotionTarget, settings config.MotionSettings, at int64, position float64, direction int, velocity float64) (MotionTarget, float64) {
	if target.prepared == nil {
		compiled, err := FlowTarget(*target.Flow, settings)
		if err != nil {
			return target, chooseNearestPhase(target, settings, position, direction, velocity)
		}
		target.prepared = compiled.prepared
		target.PatternID, target.PatternName = PatternID(compiled.prepared.id), compiled.prepared.name
	}
	if phase, ok := freestyleContinuation(previous, target, at); ok {
		return target, phase
	}
	return target, chooseNearestPhase(target, settings, position, direction, velocity)
}
