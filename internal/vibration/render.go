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

// Rendering constants. The smoothing, the speed scale and the user's timing
// offset are tuned together, because smoothing, radio and motor lag add up.
// The first physical test found the 250 ms window and release left following
// speed close to constant: about one motor step per stroke (ADR 0063).
const (
	// SpeedWindowMillis is the look-back window for averaged absolute
	// velocity, one pacer tick, so each stroke's own speed comes through.
	// Averaging |dp| over the window keeps a reversal from cancelling, which
	// an endpoint difference would.
	SpeedWindowMillis = 100
	speedSamples      = 6
	// speedScale is v0 in full spans per second for the saturating curve
	// used when no speed band is known.
	speedScale = 1.2
	// Faster attack than release: the level rises with a new stroke and dips
	// through a turnaround without stalling the motor.
	attackMillis  = 40
	releaseMillis = 100
	depthGamma    = 1.5
	// FadeMillis is the fade-in at every Play and Resume.
	FadeMillis = 2000
	// threshold is the drive below which output is exactly zero.
	threshold = 0.02
	// hysteresisSteps keeps a level near a step boundary from alternating.
	hysteresisSteps = 0.75
)

// Profile is the user's conversion profile. SpeedLow and SpeedHigh are the
// averaged carriage speeds, in full spans per second, at the user's slowest
// and fastest saved Handy speeds; zero keeps the saturating curve.
type Profile struct {
	Follow    string
	DepthEnd  string
	SpeedLow  float64
	SpeedHigh float64
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
	first, exists := timeline.Start()
	if !exists {
		r.primed = false
		return 0, false
	}
	// Pre-roll and a positive timing offset precede the first point. There is
	// no output yet, but the already-buffered stream is healthy. This does not
	// forgive an exhausted tail or invent a position outside the frame.
	if streamMillis < float64(first) && float64(first) <= r.fadeFrom {
		r.primed = false
		return 0, true
	}
	from := streamMillis
	if r.profile.Follow == FollowSpeed {
		from = math.Max(streamMillis-SpeedWindowMillis, r.fadeFrom)
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

// DriveEnvelope reads engine-sampled drive, keeping the same coverage and
// startup fade. The conversion profile does not reshape an independent lane.
func (r *Renderer) DriveEnvelope(timeline *companion.Timeline, at float64) (float64, bool) {
	first, exists := timeline.Start()
	if !exists {
		return 0, false
	}
	if at < float64(first) && float64(first) <= r.fadeFrom {
		return 0, true
	}
	value, covered := timeline.At(at)
	if !covered {
		return 0, false
	}
	return clamp(value/100*r.fade(at), 0, 1), true
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
		return speedDrive(averageSpeed(timeline, streamMillis), r.profile)
	}
}

// speedDrive spans the user's speed band on the scale every companion output
// shares, so the Handy's whole speed range is felt instead of saturating near
// its middle (ADRs 0061 and 0063).
func speedDrive(speed float64, profile Profile) float64 {
	if drive, ok := companion.SpeedBandDrive(speed, profile.SpeedLow, profile.SpeedHigh); ok {
		return drive
	}
	if speed <= 0 {
		return 0
	}
	return 1 - math.Exp(-speed/speedScale)
}

// averageSpeed is the mean absolute velocity over the window ending at
// streamMillis, in full spans per second.
func averageSpeed(timeline *companion.Timeline, streamMillis float64) float64 {
	step := float64(SpeedWindowMillis) / speedSamples
	first, _ := timeline.Start()
	previous, _ := timeline.At(math.Max(streamMillis-SpeedWindowMillis, float64(first)))
	travel := 0.0
	for index := 1; index <= speedSamples; index++ {
		position, _ := timeline.At(math.Max(streamMillis-SpeedWindowMillis+step*float64(index), float64(first)))
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

// SpeedPercentLevel places a commanded speed within the saved speed band on the
// same scale as following speed: companion.SpeedBandFloor at the slowest saved
// speed up to 1 at the fastest. It is The Handy's intensity for profiles that
// do not follow speed.
func SpeedPercentLevel(speed, low, high int) float64 {
	if speed <= 0 || high <= low {
		return 1
	}
	return companion.SpeedBandFloor + (1-companion.SpeedBandFloor)*clamp(float64(speed-low)/float64(high-low), 0, 1)
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
// gets its nearest step at or below it, never above. A positive level keeps
// at least the lowest step the ceiling allows, so a turnaround's dip does not
// switch the motor off and on with every stroke (ADR 0063); only a zero level
// is silent.
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
	return max(min(1, highest), min(step, highest))
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
