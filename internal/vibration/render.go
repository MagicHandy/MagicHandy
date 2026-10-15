// Package vibration converts the shared motion frame into a vibrator drive
// level. It is a pure function of the frame, the user's profile and its own
// smoothing state: it never invents motion, never reshapes the frame, and
// never raises a level above the user's live intensity.
package vibration

import (
	"math"

	"github.com/mapledaemon/MagicHandy/internal/companion"
)

// Follow profiles: what the vibration level follows.
const (
	FollowSpeed  = "speed"
	FollowDepth  = "depth"
	FollowSteady = "steady"
)

// Depth ends in the frame's semantic coordinates: base is position 0 and tip
// is position 100. The stroker's reverse-direction setting corrects how a
// stroker is mounted and does not change them.
const (
	DepthBase = "base"
	DepthTip  = "tip"
)

// Rendering constants. They are starting points, not measured values: the
// smoothing, the speed scale and the user's timing offset are tuned together
// during hardware acceptance, because smoothing, radio and motor lag add up.
const (
	// SpeedWindowMillis is the look-back window for averaged absolute
	// velocity. Averaging |dp| over the window keeps a reversal from
	// cancelling, which an endpoint difference would.
	SpeedWindowMillis = 250
	speedSamples      = 6
	// speedScale is v0 in full spans per second for the saturating curve.
	speedScale = 1.2
	// Faster attack than release: the level rises with a new stroke and dips
	// smoothly through a turnaround instead of stalling the motor.
	attackMillis  = 80
	releaseMillis = 250
	depthGamma    = 1.5
	// FadeMillis is the fade-in at every Play and Resume.
	FadeMillis = 2000
	// threshold is the drive below which output is exactly zero.
	threshold = 0.02
	// hysteresisSteps keeps a level near a step boundary from alternating.
	hysteresisSteps = 0.75
)

// Profile is the user's conversion profile.
type Profile struct {
	Follow   string
	DepthEnd string
}

// Renderer converts the frame into a drive fraction in 0..1.
type Renderer struct {
	profile    Profile
	smoothed   float64
	lastMillis float64
	primed     bool
	fadeFrom   float64
}

// NewRenderer returns a renderer for profile.
func NewRenderer(profile Profile) *Renderer {
	return &Renderer{profile: profile}
}

// SetProfile applies a new profile from the next level on.
func (r *Renderer) SetProfile(profile Profile) {
	r.profile = profile
}

// Restart begins a new or resumed stream at streamMillis: smoothing resets
// and output fades in from zero.
func (r *Renderer) Restart(streamMillis float64) {
	r.smoothed = 0
	r.primed = false
	r.fadeFrom = streamMillis
}

// Drive returns the drive fraction at streamMillis and whether the frame
// covered it. Uncovered time is silent: the owner reports starvation instead
// of holding the last level.
func (r *Renderer) Drive(timeline *companion.Timeline, streamMillis float64) (float64, bool) {
	from := streamMillis
	if r.profile.Follow == FollowSpeed {
		from -= SpeedWindowMillis
	}
	if !timeline.Covers(from, streamMillis) {
		r.primed = false
		return 0, false
	}
	raw := r.raw(timeline, streamMillis)
	drive := raw
	if r.profile.Follow != FollowSteady {
		drive = r.smooth(raw, streamMillis)
	}
	drive *= r.fade(streamMillis)
	if drive < threshold {
		return 0, true
	}
	return math.Min(drive, 1), true
}

func (r *Renderer) raw(timeline *companion.Timeline, streamMillis float64) float64 {
	switch r.profile.Follow {
	case FollowDepth:
		position, _ := timeline.At(streamMillis)
		toward := position / 100
		if r.profile.DepthEnd != DepthTip {
			toward = 1 - toward
		}
		return math.Pow(clamp(toward, 0, 1), depthGamma)
	case FollowSteady:
		return 1
	default:
		return 1 - math.Exp(-averageSpeed(timeline, streamMillis)/speedScale)
	}
}

// averageSpeed is the mean absolute velocity over the window ending at
// streamMillis, in full spans per second.
func averageSpeed(timeline *companion.Timeline, streamMillis float64) float64 {
	step := float64(SpeedWindowMillis) / speedSamples
	previous, _ := timeline.At(streamMillis - SpeedWindowMillis)
	travel := 0.0
	for index := 1; index <= speedSamples; index++ {
		position, _ := timeline.At(streamMillis - SpeedWindowMillis + step*float64(index))
		travel += math.Abs(position - previous)
		previous = position
	}
	return travel / 100 / (SpeedWindowMillis / 1000.0)
}

// smooth applies an elapsed-time exponential filter with a faster attack
// than release, so it behaves the same at any pacer cadence.
func (r *Renderer) smooth(raw, streamMillis float64) float64 {
	if !r.primed {
		r.primed = true
		r.lastMillis = streamMillis
		r.smoothed = raw
		return raw
	}
	elapsed := math.Max(0, streamMillis-r.lastMillis)
	r.lastMillis = streamMillis
	tau := float64(releaseMillis)
	if raw > r.smoothed {
		tau = attackMillis
	}
	r.smoothed += (1 - math.Exp(-elapsed/tau)) * (raw - r.smoothed)
	return r.smoothed
}

func (r *Renderer) fade(streamMillis float64) float64 {
	return clamp((streamMillis-r.fadeFrom)/FadeMillis, 0, 1)
}

// Output maps a drive fraction into the range from the minimum up to the
// live intensity. At an intensity of zero the vibrator is silent whatever
// the motion does, and a zero drive is always exactly zero.
func Output(drive, minimum, intensity float64) float64 {
	if drive <= 0 || intensity <= 0 {
		return 0
	}
	low := math.Min(clamp(minimum, 0, 1), intensity)
	return low + clamp(drive, 0, 1)*(intensity-low)
}

// Quantize picks the actuator step for level with steps steps. last is the
// step sent before, or -1. The ceiling quantizes downward: a coarse motor
// gets its nearest step at or below it, never above.
func Quantize(level float64, steps, last int, ceiling float64) int {
	if level <= 0 || steps <= 0 {
		return 0
	}
	target := clamp(level, 0, 1) * float64(steps)
	step := last
	if last < 0 || math.Abs(target-float64(last)) > hysteresisSteps {
		step = int(math.Round(target))
	}
	highest := int(math.Floor(clamp(ceiling, 0, 1)*float64(steps) + 1e-9))
	return max(0, min(step, highest))
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
