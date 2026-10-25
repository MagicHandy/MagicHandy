package transport

import (
	"errors"
	"math"

	"github.com/mapledaemon/MagicHandy/internal/companion"
)

// The engine supplies already-sampled drive. The owner never creates a pattern.
func vibrationFrameTimeline(frame *VibrationFrame) (companion.Timeline, error) {
	var next companion.Timeline
	if frame == nil {
		return next, nil
	}
	if len(frame.Points) > 64 || (!frame.Follow && len(frame.Points) < 2) || (frame.Follow && len(frame.Points) != 0) {
		return next, errors.New("invalid vibration frame size")
	}
	points := make([]companion.Point, len(frame.Points))
	for i, point := range frame.Points {
		if math.IsNaN(point.LevelPercent) || math.IsInf(point.LevelPercent, 0) || point.LevelPercent < 0 || point.LevelPercent > 100 {
			return next, errors.New("vibration frame level must be finite and within 0..100")
		}
		if point.TimeMillis < 0 || (i > 0 && point.TimeMillis <= frame.Points[i-1].TimeMillis) {
			return next, errors.New("vibration frame times must be nonnegative and strictly increasing")
		}
		points[i] = companion.Point{TimeMillis: point.TimeMillis, Position: point.LevelPercent}
	}
	if !frame.Follow {
		if err := next.Append(points); err != nil {
			return next, err
		}
	}
	return next, nil
}

func (v *IntifaceVibrator) applyVibrationFrameLocked(frame *VibrationFrame, next companion.Timeline) {
	if frame == nil {
		return
	}
	if frame.Revision != v.stream.vibrationRevision {
		v.invalidateOutputLocked()
		v.addEventLocked("envelope_changed", 0, nil)
	}
	v.stream.vibrationRevision = frame.Revision
	v.stream.independent = !frame.Follow
	v.stream.vibration = next
	v.signalLocked()
}
