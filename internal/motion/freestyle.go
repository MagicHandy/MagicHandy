package motion

import (
	"errors"
	"math"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// FreestyleSpec is one window of Freestyle's endless generated stroke stream.
// Every stroke is a pure function of the stream seed, its index and the
// keyframed controls, so two windows that overlap produce identical strokes
// there and a continuation has nothing to blend. Windows never loop: the mode
// manager continues the stream well before a window ends, and an unattended
// window finishes and stops like any other finite program.
//
// The stream is semantic content for the shared engine. It has no timers, no
// transport access and no device coordinates; the ordinary plan, sampler,
// sanitizer and Stop own playback.
type FreestyleSpec struct {
	// FromStroke is where the stream starts from rest. Strokes before it rest
	// at its lower turn, so a fresh start eases in instead of joining a stroke
	// already in flight.
	FromStroke int `json:"from_stroke"`
	// StartStroke is the stream index of this window's first stroke.
	StartStroke int `json:"start_stroke"`
	// Strokes is how many strokes this window plays.
	Strokes int `json:"strokes"`
	// EndStroke, when positive, ends the stream: the strokes before it wind
	// down and the stream comes to rest at its lower turn.
	EndStroke int `json:"end_stroke,omitempty"`
	// MinSpeedPercent is the slowest pace; FlowSpec.SpeedPercent is the fastest.
	MinSpeedPercent int `json:"min_speed_percent"`
	// Keyframes ramp the controls along the stream. The first is the starting
	// state; each later one starts a ramp at its stroke.
	Keyframes []FreestyleKeyframe `json:"keyframes"`
}

// FreestyleKeyframe starts a ramp at a stroke, from wherever the controls are
// there to new values reached RampStrokes strokes later. Strokes before it are
// unaffected, so adding a keyframe ahead of the playhead never rewrites motion
// that is already queued.
type FreestyleKeyframe struct {
	Stroke      int `json:"stroke"`
	RampStrokes int `json:"ramp_strokes,omitempty"`
	// Linear ramps join into a steady trend, as a session shape needs; other
	// ramps ease in and out, as a control change should.
	Linear   bool              `json:"linear,omitempty"`
	Controls FreestyleControls `json:"controls"`
}

// FreestyleControls are tendencies of the generated stream, not a pattern.
// Percentages run 0-100 except AccentPercent, which runs from -100 (faster
// toward the base) through 0 (even) to 100 (faster toward the tip).
type FreestyleControls struct {
	// PacePercent places the pace inside the speed band.
	PacePercent int `json:"pace_percent"`
	// LengthPercent is the typical stroke length, short to full.
	LengthPercent int `json:"length_percent"`
	// FocusPercent is where strokes gather, 0 the base and 100 the tip.
	FocusPercent int `json:"focus_percent"`
	// RoamingPercent is how far the working region wanders from the focus.
	RoamingPercent int `json:"roaming_percent"`
	// VarietyPercent is how much pace, length and texture evolve; 0 repeats
	// one even stroke.
	VarietyPercent int `json:"variety_percent"`
	AccentPercent  int `json:"accent_percent"`
	// EnergyPercent scales the pace for a session shape: 100 is the chosen
	// pace and 0 the bottom of the speed band.
	EnergyPercent int `json:"energy_percent"`
	// TeasePercent draws strokes toward short, unhurried work at the tip.
	TeasePercent int `json:"tease_percent"`
}

const (
	// FreestyleMaximumWindowStrokes keeps a window inside the curve point
	// budget with room for the rests a lively stream inserts.
	FreestyleMaximumWindowStrokes = 160
	// FreestyleMaximumKeyframes bounds one window's control history; the mode
	// manager compacts settled ramps well before reaching it.
	FreestyleMaximumKeyframes = 48
	// FreestyleMaximumRampStrokes bounds one ramp.
	FreestyleMaximumRampStrokes = 1024
	// freestyleMaximumStroke guards stroke arithmetic, not session length: at
	// two strokes a second it is more than ten years.
	freestyleMaximumStroke = 1 << 30
)

// streamWindow is the backend-only index of a compiled Freestyle window. It
// maps stream strokes to window time for continuations and progress.
type streamWindow struct {
	seed        uint32
	startStroke int
	endStroke   int
	// strokeStarts holds the curve time at which each played stroke starts,
	// followed by the window duration.
	strokeStarts []int64
}

func (s FlowSpec) validateFreestyle(settings config.MotionSettings) error {
	f := s.Freestyle
	if f == nil {
		return nil
	}
	if s.Gesture != nil || s.Strokes != nil || len(s.Steps) != 0 || len(s.Layers) != 0 || s.LoopCycles != 0 {
		return errors.New("a freestyle stream has no gesture, stroke score, sections, layers or loop")
	}
	if f.FromStroke < 0 || f.StartStroke < f.FromStroke || f.StartStroke > freestyleMaximumStroke ||
		f.Strokes < 1 || f.Strokes > FreestyleMaximumWindowStrokes {
		return errors.New("a freestyle window starts at or after its stream start and plays 1-160 strokes")
	}
	if f.EndStroke != 0 && (f.EndStroke <= f.FromStroke || f.StartStroke+f.Strokes > f.EndStroke) {
		return errors.New("a freestyle window cannot play past the stream's end")
	}
	if f.MinSpeedPercent < normalizeMotionSettings(settings).SpeedMinPercent || f.MinSpeedPercent > s.SpeedPercent {
		return errors.New("freestyle minimum speed must stay inside the saved speed limits and below its maximum")
	}
	return validateFreestyleKeyframes(f.Keyframes)
}

func validateFreestyleKeyframes(keyframes []FreestyleKeyframe) error {
	if len(keyframes) == 0 || len(keyframes) > FreestyleMaximumKeyframes {
		return errors.New("a freestyle stream needs 1-48 control keyframes")
	}
	for index, keyframe := range keyframes {
		if err := keyframe.Controls.validate(); err != nil {
			return err
		}
		if keyframe.Stroke < 0 || keyframe.Stroke > freestyleMaximumStroke {
			return errors.New("freestyle keyframe strokes must stay inside the stream bounds")
		}
		if index == 0 {
			continue
		}
		if keyframe.Stroke <= keyframes[index-1].Stroke ||
			keyframe.RampStrokes < 1 || keyframe.RampStrokes > FreestyleMaximumRampStrokes {
			return errors.New("freestyle keyframes need increasing strokes and a ramp of 1-1024 strokes")
		}
	}
	return nil
}

func (c FreestyleControls) validate() error {
	for _, value := range []int{c.PacePercent, c.LengthPercent, c.FocusPercent, c.RoamingPercent,
		c.VarietyPercent, c.EnergyPercent, c.TeasePercent} {
		if value < 0 || value > 100 {
			return errors.New("freestyle controls must stay within 0-100")
		}
	}
	if c.AccentPercent < -100 || c.AccentPercent > 100 {
		return errors.New("freestyle accent must stay within -100 to 100")
	}
	return nil
}

// streamControls are the controls at one stroke, as fractions.
type streamControls struct {
	pace, length, focus, roaming, variety, accent, energy, tease float64
}

func (c FreestyleControls) fractions() streamControls {
	return streamControls{pace: float64(c.PacePercent) / 100, length: float64(c.LengthPercent) / 100,
		focus: float64(c.FocusPercent) / 100, roaming: float64(c.RoamingPercent) / 100,
		variety: float64(c.VarietyPercent) / 100, accent: float64(c.AccentPercent) / 100,
		energy: float64(c.EnergyPercent) / 100, tease: float64(c.TeasePercent) / 100}
}

// keyframeTrack evaluates the keyframed controls. anchors[j] is where the
// controls stood when keyframe j began its ramp.
type keyframeTrack struct {
	keyframes []FreestyleKeyframe
	targets   []streamControls
	anchors   []streamControls
}

func newKeyframeTrack(keyframes []FreestyleKeyframe) keyframeTrack {
	track := keyframeTrack{keyframes: keyframes, targets: make([]streamControls, len(keyframes)),
		anchors: make([]streamControls, len(keyframes))}
	for index, keyframe := range keyframes {
		track.targets[index] = keyframe.Controls.fractions()
		if index == 0 {
			track.anchors[0] = track.targets[0]
			continue
		}
		track.anchors[index] = track.rampValue(index-1, float64(keyframe.Stroke))
	}
	return track
}

// at returns the controls at stroke k.
func (t keyframeTrack) at(k int) streamControls {
	for index := len(t.keyframes) - 1; index > 0; index-- {
		if k > t.keyframes[index].Stroke {
			return t.rampValue(index, float64(k))
		}
	}
	return t.targets[0]
}

func (t keyframeTrack) rampValue(index int, k float64) streamControls {
	if index == 0 {
		return t.targets[0]
	}
	keyframe := t.keyframes[index]
	progress := (k - float64(keyframe.Stroke)) / float64(keyframe.RampStrokes)
	weight := clampFloat(progress, 0, 1)
	if !keyframe.Linear {
		weight = smootherStep(weight)
	}
	return blendControls(t.anchors[index], t.targets[index], weight, progress)
}

// blendControls is exact at both ends of a ramp, so a settled ramp and the
// compacted keyframe that replaces it produce bit-identical strokes.
func blendControls(from, to streamControls, weight, progress float64) streamControls {
	if progress <= 0 {
		return from
	}
	if progress >= 1 {
		return to
	}
	mix := func(a, b float64) float64 { return a + (b-a)*weight }
	return streamControls{pace: mix(from.pace, to.pace), length: mix(from.length, to.length),
		focus: mix(from.focus, to.focus), roaming: mix(from.roaming, to.roaming),
		variety: mix(from.variety, to.variety), accent: mix(from.accent, to.accent),
		energy: mix(from.energy, to.energy), tease: mix(from.tease, to.tease)}
}

// CompactFreestyleKeyframes drops control history that can no longer affect
// strokes at or after fromStroke. A keyframe is folded into the starting state
// only once its ramp has settled before both fromStroke and the next ramp, so
// every stroke from fromStroke on is generated exactly as before.
func CompactFreestyleKeyframes(keyframes []FreestyleKeyframe, fromStroke int) []FreestyleKeyframe {
	if len(keyframes) == 0 {
		return nil
	}
	// Timing includes the surrounding strokes, whose events read controls at
	// the beginning of their jittered block. Keep that history too: replacing
	// it merely because a ramp settled before the playhead changes already
	// generated turns and can introduce a jump at the next continuation.
	fromStroke -= freestyleContextStrokes + freestyleBlockStrokes + 6
	first := 0
	for index := 1; index < len(keyframes); index++ {
		settled := keyframes[index].Stroke + keyframes[index].RampStrokes
		if settled > fromStroke || (index+1 < len(keyframes) && settled > keyframes[index+1].Stroke) {
			break
		}
		first = index
	}
	compacted := make([]FreestyleKeyframe, 0, len(keyframes)-first)
	compacted = append(compacted, FreestyleKeyframe{Controls: keyframes[first].Controls})
	return append(compacted, keyframes[first+1:]...)
}

// FreestyleControlsAt returns the keyframed controls at a stream stroke,
// rounded to whole percentages, so a caller can start a new ramp from where
// the stream actually is.
func FreestyleControlsAt(keyframes []FreestyleKeyframe, stroke int) FreestyleControls {
	if len(keyframes) == 0 {
		return FreestyleControls{}
	}
	c := newKeyframeTrack(keyframes).at(stroke)
	percent := func(value float64) int { return int(math.Round(value * 100)) }
	return FreestyleControls{PacePercent: percent(c.pace), LengthPercent: percent(c.length),
		FocusPercent: percent(c.focus), RoamingPercent: percent(c.roaming), VarietyPercent: percent(c.variety),
		AccentPercent: percent(c.accent), EnergyPercent: percent(c.energy), TeasePercent: percent(c.tease)}
}

func cloneFreestyleSpec(spec *FreestyleSpec) *FreestyleSpec {
	if spec == nil {
		return nil
	}
	cloned := *spec
	cloned.Keyframes = append([]FreestyleKeyframe(nil), spec.Keyframes...)
	return &cloned
}

// NewFreestyleFlow wraps one Freestyle window in the flow contract the engine
// compiles. The carrier fields are fixed; the window carries everything else.
// The seed identifies the stream and must not be zero.
func NewFreestyleFlow(seed uint32, maxSpeedPercent int, window FreestyleSpec) FlowSpec {
	return FlowSpec{MinPercent: 0, MaxPercent: 100, SpeedPercent: maxSpeedPercent, RangeFloorPercent: 10,
		MemoryCycles: 8, Seed: seed, Freestyle: cloneFreestyleSpec(&window)}
}
