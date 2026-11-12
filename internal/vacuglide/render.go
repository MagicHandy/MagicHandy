// Package vacuglide renders the VacuGlide's controls from the shared motion
// frame (ADR 0069). The VacuGlide has no position input: a crank-driven piston
// moves air that pulls and pushes its sleeve, so the motor sets the pace, and
// the air sealed in the sleeve sets how long and tight each stroke is. The
// motor follows how fast the frame moves; an optional follower reads how far
// the frame's strokes reach and steps the stroke valves toward it.
package vacuglide

import (
	"math"

	"github.com/mapledaemon/MagicHandy/internal/companion"
)

const (
	// FullSpeedTravel is Autoblow's own scale: a script moving 600 position
	// units per second plays at 100% motor speed.
	FullSpeedTravel = 600.0
	// FloorPercent keeps a moving frame above the motor's stall point, about
	// 4% on Autoblow's scale.
	FloorPercent = 5.0
	// HoldPercent is the pace below which the frame is holding still, so the
	// motor stops instead of stalling.
	HoldPercent = 1.0
	// SpeedWindowMillis averages pace over about one stroke: the VacuGlide
	// makes its own strokes, so it follows pace, not each turnaround.
	SpeedWindowMillis = 1000.0
	// ReachWindowMillis looks back far enough to include a whole slow stroke.
	ReachWindowMillis = 2500.0
	attackMillis      = 250.0
	releaseMillis     = 600.0
)

// Profile scales motor speed. Scale 1 keeps the pace the frame would have on
// The Handy; a larger scale stretches MagicHandy's top speed toward the
// motor's.
type Profile struct {
	Scale float64
}

// Renderer turns the frame into a motor speed and a stroke reach. It keeps
// only smoothing state; it never reshapes the frame.
type Renderer struct {
	profile    Profile
	smoothed   float64
	lastMillis float64
	primed     bool
}

// NewRenderer returns a renderer for profile.
func NewRenderer(profile Profile) *Renderer {
	renderer := &Renderer{}
	renderer.SetProfile(profile)
	return renderer
}

// SetProfile applies a new scale from the next sample.
func (r *Renderer) SetProfile(profile Profile) {
	if !(profile.Scale > 0) || math.IsInf(profile.Scale, 0) {
		profile.Scale = 1
	}
	r.profile = profile
}

// Restart forgets smoothing for a new or resumed stream.
func (r *Renderer) Restart() {
	r.primed = false
	r.smoothed = 0
}

// Speed is the motor speed in percent at stream time at: zero while the frame
// holds still, and at least FloorPercent while it moves. It reports false
// where the frame does not cover at, so the owner stops the motor there.
func (r *Renderer) Speed(timeline *companion.Timeline, at float64) (float64, bool) {
	if _, covered := timeline.At(at); !covered {
		return 0, false
	}
	first, _ := timeline.Start()
	from := math.Max(at-SpeedWindowMillis, float64(first))
	span := at - from
	raw := 0.0
	if span > 0 {
		rate := timeline.Travel(from, at) / (span / 1000)
		raw = rate / FullSpeedTravel * 100 * r.profile.Scale
	}
	return MotorPercent(r.smooth(raw, at)), true
}

// MotorPercent applies the stall floor and the hold: what the motor is told.
func MotorPercent(pace float64) float64 {
	switch {
	case !(pace >= HoldPercent):
		return 0
	case pace < FloorPercent:
		return FloorPercent
	default:
		return math.Min(100, pace)
	}
}

// Reach is how far toward the base the frame's strokes reached over the last
// ReachWindowMillis, in percent: 100 means strokes reach the base, 0 that
// they stay at the tip. Position 0 is the base end of the frame.
func (r *Renderer) Reach(timeline *companion.Timeline, at float64) (float64, bool) {
	first, ok := timeline.Start()
	if !ok {
		return 0, false
	}
	lowest, covered := timeline.Lowest(math.Max(at-ReachWindowMillis, float64(first)), at)
	if !covered {
		return 0, false
	}
	return 100 - lowest, true
}

// smooth follows a rising pace faster than a falling one, by elapsed stream
// time, so the motor does not hunt between strokes.
func (r *Renderer) smooth(raw, at float64) float64 {
	if !r.primed {
		r.primed, r.smoothed, r.lastMillis = true, raw, at
		return raw
	}
	elapsed := math.Max(0, at-r.lastMillis)
	r.lastMillis = at
	constant := releaseMillis
	if raw > r.smoothed {
		constant = attackMillis
	}
	r.smoothed += (raw - r.smoothed) * (1 - math.Exp(-elapsed/constant))
	return r.smoothed
}
