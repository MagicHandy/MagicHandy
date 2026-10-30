package companion

import "math"

// SpeedBandFloor is the drive at the user's slowest saved Handy speed. Slower
// motion fades linearly to zero, so a held position is silent.
const SpeedBandFloor = 0.20

// SpeedBandDrive places a carriage speed, in full spans per second, on the
// user's saved Handy speed band: SpeedBandFloor at the slowest saved speed up
// to 1 at the fastest (ADRs 0061 and 0063). The vibrator and the Coyote follow
// speed on this one scale. ok is false when no band is known, and each output
// keeps its own fallback.
func SpeedBandDrive(speed, low, high float64) (drive float64, ok bool) {
	switch {
	case low <= 0:
		return 0, false
	case speed <= 0:
		return 0, true
	case speed < low:
		return SpeedBandFloor * speed / low, true
	case high <= low:
		return 1, true
	default:
		return SpeedBandFloor + (1-SpeedBandFloor)*math.Max(0, math.Min(1, (speed-low)/(high-low))), true
	}
}
