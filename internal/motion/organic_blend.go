package motion

import (
	"math"
	"math/rand"
)

const (
	crossfadeMinDurationMS = 500
	crossfadeMaxDurationMS = 1000
	crossfadeMinPoints     = 3
	crossfadeMaxPoints     = 5
)

// MotionBlendState is the instantaneous device pose used for spline blending.
type MotionBlendState struct {
	Position float64 // 0–100 axis percent
	Velocity float64 // percent per second
}

// CrossfadeOptions tunes Hermite transition buffers.
type CrossfadeOptions struct {
	DurationMS         int
	PointCount         int
	HardwareSafetyLock bool
}

// DefaultCrossfadeOptions returns a human-paced 750ms / 4-point blend.
func DefaultCrossfadeOptions() CrossfadeOptions {
	return CrossfadeOptions{
		DurationMS: 750,
		PointCount: 4,
	}
}

// CrossfadeOptionsForPhysics picks blend pacing suited to the stroke type.
func CrossfadeOptionsForPhysics(physics ChaoticPhysics) CrossfadeOptions {
	if IsTurboTipo(physics.TipoBatida) {
		return CrossfadeOptions{
			DurationMS: 4,
			PointCount: 2,
		}
	}
	return DefaultCrossfadeOptions()
}

// EstimateVelocityFromWaypoints derives percent-per-second from the first motion step.
func EstimateVelocityFromWaypoints(stream []ChaosWaypoint) float64 {
	if len(stream) < 2 {
		return 0
	}
	dt := float64(stream[1].TimeDelta)
	if dt <= 0 {
		return 0
	}
	dp := float64(stream[1].Position - stream[0].Position)
	vel := dp / (dt / 1000.0)
	const maxVel = 80.0
	if vel > maxVel {
		return maxVel
	}
	if vel < -maxVel {
		return -maxVel
	}
	return vel
}

// CubicHermiteCrossfade builds micro-waypoints between two blend states.
func CubicHermiteCrossfade(
	from, to MotionBlendState,
	opts CrossfadeOptions,
	rng *rand.Rand,
) []ChaosWaypoint {
	opts = normalizeCrossfadeOptions(opts)
	if rng == nil {
		rng = rand.New(rand.NewSource(1)) // #nosec G404
	}

	p0 := from.Position
	p1 := to.Position
	scale := float64(opts.DurationMS) / 3.0
	m0 := from.Velocity * scale / 1000.0 * float64(opts.DurationMS)
	m1 := to.Velocity * scale / 1000.0 * float64(opts.DurationMS)

	points := opts.PointCount
	if points < crossfadeMinPoints {
		points = crossfadeMinPoints
	}
	segmentMS := opts.DurationMS / (points + 1)
	if segmentMS < 1 {
		segmentMS = 1
	}

	out := make([]ChaosWaypoint, 0, points+1)
	anchorDelta := segmentMS / 2
	if anchorDelta < 1 {
		anchorDelta = 1
	}
	out = append(out, ChaosWaypoint{
		TimeDelta: clampDelta(anchorDelta, opts.HardwareSafetyLock),
		Position:  int(math.Round(clampFloat(p0, 0, 100))),
	})

	prevPos := p0
	crossfadeMaxStep := 12
	for i := 1; i <= points; i++ {
		t := float64(i) / float64(points)
		t2 := t * t
		t3 := t2 * t
		h00 := 2*t3 - 3*t2 + 1
		h10 := t3 - 2*t2 + t
		h01 := -2*t3 + 3*t2
		h11 := t3 - t2
		pos := h00*p0 + h10*m0 + h01*p1 + h11*m1

		delta := segmentMS
		if rng != nil {
			jitter := 1 + (rng.Float64()*2-1)*0.09
			delta = int(math.Round(float64(segmentMS) * jitter))
		}
		delta = clampDelta(delta, opts.HardwareSafetyLock)
		if delta < 1 {
			delta = 1
		}

		posI := int(math.Round(clampFloat(pos, 0, 100)))
		posI = clampOrganicStep(int(math.Round(prevPos)), posI, crossfadeMaxStep)
		posI = enforceMinOrganicStep(int(math.Round(prevPos)), posI, 0, 100)

		out = append(out, ChaosWaypoint{TimeDelta: delta, Position: posI})
		prevPos = float64(posI)
	}
	return out
}

// StitchWithCrossfade prepends a Hermite buffer from `from` into `stream` without duplicating the anchor.
func StitchWithCrossfade(
	from MotionBlendState,
	stream []ChaosWaypoint,
	opts CrossfadeOptions,
	rng *rand.Rand,
) []ChaosWaypoint {
	if len(stream) == 0 {
		return stream
	}
	targetPos := float64(stream[0].Position)
	targetVel := EstimateVelocityFromWaypoints(stream) * 0.45
	to := MotionBlendState{Position: targetPos, Velocity: targetVel}
	crossfade := CubicHermiteCrossfade(from, to, opts, rng)
	if len(crossfade) == 0 {
		return stream
	}
	tail := crossfade[len(crossfade)-1].Position
	head := stream[0].Position
	if absIntValue(tail-head) <= 2 {
		return append(crossfade, stream[1:]...)
	}
	return append(crossfade, stream...)
}

// StitchFromPosition crossfades from a device position into a new organic stream.
func StitchFromPosition(
	continueFrom int,
	stream []ChaosWaypoint,
	opts CrossfadeOptions,
	rng *rand.Rand,
) []ChaosWaypoint {
	if continueFrom < 0 || len(stream) == 0 {
		return stream
	}
	from := MotionBlendState{
		Position: float64(continueFrom),
		Velocity: 0,
	}
	if continueFrom != stream[0].Position {
		return StitchWithCrossfade(from, stream, opts, rng)
	}
	// Already anchored — still add a short ease-in if velocity jump is large.
	targetVel := EstimateVelocityFromWaypoints(stream)
	if math.Abs(targetVel) < 8 {
		return stream
	}
	short := opts
	short.DurationMS = clampInt(short.DurationMS/2, crossfadeMinDurationMS/2, crossfadeMaxDurationMS/2)
	short.PointCount = crossfadeMinPoints
	return StitchWithCrossfade(from, stream, short, rng)
}

func normalizeCrossfadeOptions(opts CrossfadeOptions) CrossfadeOptions {
	fast := opts.DurationMS > 0 && opts.DurationMS <= 10
	if !fast {
		if opts.DurationMS < crossfadeMinDurationMS {
			opts.DurationMS = crossfadeMinDurationMS
		}
		if opts.DurationMS > crossfadeMaxDurationMS {
			opts.DurationMS = crossfadeMaxDurationMS
		}
	} else if opts.DurationMS < 1 {
		opts.DurationMS = 1
	}
	if opts.PointCount < crossfadeMinPoints {
		opts.PointCount = crossfadeMinPoints
	}
	if opts.PointCount > crossfadeMaxPoints {
		opts.PointCount = crossfadeMaxPoints
	}
	return opts
}
