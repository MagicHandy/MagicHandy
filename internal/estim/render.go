package estim

import (
	"math"
)

// Follow modes of the conversion profile. None of them maps position to
// channel strength: strength stays where the user set it, and the profile only
// shapes pulse width, channel distribution and pulse frequency beneath it.
const (
	// FollowStroke distributes output between the channels by position and
	// sets pulse width from movement speed.
	FollowStroke = "stroke"
	// FollowSpeed sets pulse width from movement speed, equally on both
	// channels.
	FollowSpeed = "speed"
	// FollowPosition distributes a steady pulse width by position.
	FollowPosition = "position"
)

// Profile is the user's conversion profile. Frequencies are pulse rates in
// Hz; DG-LAB's period form is derived at the boundary.
type Profile struct {
	Follow       string
	MinHz        int
	MaxHz        int
	Balance      int
	SwapChannels bool
}

// Rendering constants. Speeds are in full spans per second; times in stream
// milliseconds.
const (
	widthFloor          = 0.15
	positionWidth       = 0.6
	channelFloor        = 0.25
	speedScale          = 1.5
	attackMillis        = 60.0
	releaseMillis       = 250.0
	tempoMillis         = 1500.0
	fadeInMillis        = 2000.0
	frequencyStepHz     = 5
	frequencyHoldMillis = 500.0
	halfFrameMillis     = FrameMillis / 2.0
	maxPulseRateHz      = 1000 / MinPeriodMillis
	minPulseRateHz      = 1
)

// Renderer converts the shared frame into pulse frames. It is a pure,
// deterministic function of the timeline, the profile and its own smoothing
// state; it never invents motion and never changes channel strength.
type Renderer struct {
	profile   Profile
	envelope  float64
	tempo     float64
	hz        int
	holdUntil float64
	fadeStart float64
}

// NewRenderer returns a renderer for profile.
func NewRenderer(profile Profile) *Renderer {
	renderer := &Renderer{}
	renderer.SetProfile(profile)
	return renderer
}

// SetProfile applies a new profile from the next packet on.
func (r *Renderer) SetProfile(profile Profile) {
	r.profile = NormalizeProfile(profile)
	r.hz = 0
}

// NormalizeProfile clamps a profile into its valid ranges.
func NormalizeProfile(profile Profile) Profile {
	switch profile.Follow {
	case FollowStroke, FollowSpeed, FollowPosition:
	default:
		profile.Follow = FollowStroke
	}
	profile.MinHz = clampInt(profile.MinHz, minPulseRateHz, maxPulseRateHz)
	profile.MaxHz = clampInt(profile.MaxHz, profile.MinHz, maxPulseRateHz)
	profile.Balance = clampInt(profile.Balance, -100, 100)
	return profile
}

// Restart begins a new stream: smoothing resets and output fades in from
// streamMillis, so playback never starts at full pulse width.
func (r *Renderer) Restart(streamMillis float64) {
	r.envelope = 0
	r.tempo = 0
	r.hz = 0
	r.holdUntil = 0
	r.fadeStart = streamMillis
}

// Packet renders the four frames starting at startMillis and reports how many
// of them the timeline covered. Uncovered frames are silent.
func (r *Renderer) Packet(timeline *Timeline, startMillis float64) ([FramesPerPacket]Frame, int) {
	var frames [FramesPerPacket]Frame
	covered := 0
	period := r.period(startMillis)
	for index := range frames {
		center := startMillis + float64(index*FrameMillis) + halfFrameMillis
		position, speed, ok := sample(timeline, center)
		r.smooth(speed, ok)
		frames[index] = SilentFrame
		frames[index].A.PeriodMillis, frames[index].B.PeriodMillis = period, period
		if !ok {
			continue
		}
		covered++
		gainA, gainB := r.gains(position)
		width := r.width() * r.fade(center)
		frames[index].A.Intensity = intensity(width * gainA)
		frames[index].B.Intensity = intensity(width * gainB)
		if r.profile.SwapChannels {
			frames[index].A, frames[index].B = frames[index].B, frames[index].A
		}
	}
	return frames, covered
}

func sample(timeline *Timeline, center float64) (position float64, speed float64, ok bool) {
	position, ok = timeline.At(center)
	if !ok {
		return 0, 0, false
	}
	before, beforeOK := timeline.At(center - halfFrameMillis)
	after, afterOK := timeline.At(center + halfFrameMillis)
	span := 2 * halfFrameMillis
	switch {
	case beforeOK && afterOK:
	case afterOK:
		before, span = position, halfFrameMillis
	case beforeOK:
		after, span = position, halfFrameMillis
	default:
		return position / 100, 0, true
	}
	speed = math.Abs(after-before) / 100 / (span / 1000)
	return position / 100, speed, true
}

func (r *Renderer) smooth(speed float64, covered bool) {
	target := 0.0
	if covered {
		target = 1 - math.Exp(-speed/speedScale)
	}
	timeConstant := releaseMillis
	if target > r.envelope {
		timeConstant = attackMillis
	}
	r.envelope += (target - r.envelope) * (1 - math.Exp(-FrameMillis/timeConstant))
	r.tempo += (target - r.tempo) * (1 - math.Exp(-FrameMillis/tempoMillis))
}

func (r *Renderer) width() float64 {
	if r.profile.Follow == FollowPosition {
		return positionWidth
	}
	return widthFloor + (1-widthFloor)*r.envelope
}

func (r *Renderer) gains(position float64) (float64, float64) {
	gainA, gainB := 1.0, 1.0
	if r.profile.Follow != FollowSpeed {
		angle := clampFloat(position, 0, 1) * math.Pi / 2
		gainA = channelFloor + (1-channelFloor)*math.Cos(angle)
		gainB = channelFloor + (1-channelFloor)*math.Sin(angle)
	}
	bias := float64(r.profile.Balance) / 100
	if bias > 0 {
		gainA *= 1 - bias
	} else if bias < 0 {
		gainB *= 1 + bias
	}
	return gainA, gainB
}

func (r *Renderer) fade(center float64) float64 {
	return clampFloat((center-r.fadeStart)/fadeInMillis, 0, 1)
}

// period picks the packet's pulse period. The rate follows the movement tempo
// within the user's band, moves in 5 Hz steps and holds each step for at
// least 500 ms: DG-LAB notes that quickly changing periods longer than the
// 25 ms output window are reshaped by the device.
func (r *Renderer) period(startMillis float64) int {
	low, high := r.profile.MinHz, r.profile.MaxHz
	target := low + int(math.Round(float64(high-low)*r.tempo/frequencyStepHz))*frequencyStepHz
	target = clampInt(target, low, high)
	if r.hz == 0 || (target != r.hz && startMillis >= r.holdUntil) {
		if r.hz != 0 {
			r.holdUntil = startMillis + frequencyHoldMillis
		}
		r.hz = target
	}
	return PeriodForHz(r.hz)
}

// PeriodForHz converts a pulse rate to the nearest valid period.
func PeriodForHz(hz int) int {
	hz = clampInt(hz, minPulseRateHz, maxPulseRateHz)
	return clampInt(int(math.Round(1000/float64(hz))), MinPeriodMillis, MaxPeriodMillis)
}

// ReferencePacket renders the explicit channel test while motion is paused.
// Only the selected channel pulses, at full width: the first half at the band's
// lowest rate and the second at its highest. The opening frames ramp in. This
// reference is not a guarantee of the strongest possible physical sensation.
func ReferencePacket(profile Profile, channel Channel, index int, total int) [FramesPerPacket]Frame {
	profile = NormalizeProfile(profile)
	hz := profile.MinHz
	if index >= total/2 {
		hz = profile.MaxHz
	}
	period := PeriodForHz(hz)
	var frames [FramesPerPacket]Frame
	for frame := range frames {
		frames[frame] = SilentFrame
		frames[frame].A.PeriodMillis, frames[frame].B.PeriodMillis = period, period
		level := intensity(min(1, float64(index*FramesPerPacket+frame+1)/referenceRampFrames))
		if channel == ChannelA {
			frames[frame].A.Intensity = level
		} else {
			frames[frame].B.Intensity = level
		}
	}
	return frames
}

// ReferencePackets is the reference pulse length: 1.6 s.
const (
	ReferencePackets    = 16
	referenceRampFrames = 6.0
)

func intensity(fraction float64) int {
	return clampInt(int(math.Round(fraction*MaxIntensity)), 0, MaxIntensity)
}

func clampFloat(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
