package estim

import (
	"errors"
	"fmt"
	"sort"
)

// maxTimelinePoints bounds memory for one stream; the owner prunes points the
// pacer has passed long before this matters.
const maxTimelinePoints = 8192

var errTimelineFull = errors.New("e-stim timeline is full")

// Point is one timed semantic position in stream milliseconds. Position is
// the shared engine's 0..100 span; 0 is the base end of the stroke.
type Point struct {
	TimeMillis int64
	Position   float64
}

// Timeline holds the current stream's points. It is the owner's read-only
// copy of the shared frame: the renderer samples it and never reshapes it.
type Timeline struct {
	points []Point
}

// Append adds points in stream order. A point at or before the current end
// replaces the tail from that time, matching how the engine re-plans ahead.
func (t *Timeline) Append(points []Point) error {
	for _, point := range points {
		if point.Position < 0 || point.Position > 100 || point.TimeMillis < 0 {
			return fmt.Errorf("e-stim point %+v outside the semantic frame", point)
		}
	}
	for _, point := range points {
		if count := len(t.points); count > 0 && point.TimeMillis <= t.points[count-1].TimeMillis {
			cut := sort.Search(count, func(index int) bool { return t.points[index].TimeMillis >= point.TimeMillis })
			t.points = t.points[:cut]
		}
		if len(t.points) >= maxTimelinePoints {
			return errTimelineFull
		}
		t.points = append(t.points, point)
	}
	return nil
}

// Reset clears the timeline for a new stream.
func (t *Timeline) Reset() {
	t.points = t.points[:0]
}

// Prune drops points that end before beforeMillis while keeping the one
// segment that still spans it.
func (t *Timeline) Prune(beforeMillis int64) {
	cut := sort.Search(len(t.points), func(index int) bool { return t.points[index].TimeMillis >= beforeMillis })
	if cut > 1 {
		t.points = append(t.points[:0], t.points[cut-1:]...)
	}
}

// End reports the last covered stream time.
func (t *Timeline) End() (int64, bool) {
	if len(t.points) == 0 {
		return 0, false
	}
	return t.points[len(t.points)-1].TimeMillis, true
}

// Len reports the number of retained points.
func (t *Timeline) Len() int {
	return len(t.points)
}

// At samples the frame by linear interpolation, which is how the shared
// engine's timed points are played between knots. It reports false outside
// the covered interval: the renderer outputs silence there instead of holding
// the last value.
func (t *Timeline) At(millis float64) (float64, bool) {
	count := len(t.points)
	if count == 0 || millis < float64(t.points[0].TimeMillis) || millis > float64(t.points[count-1].TimeMillis) {
		return 0, false
	}
	index := sort.Search(count, func(index int) bool { return float64(t.points[index].TimeMillis) >= millis })
	if index == 0 {
		return t.points[0].Position, true
	}
	previous, next := t.points[index-1], t.points[index]
	span := float64(next.TimeMillis - previous.TimeMillis)
	if span <= 0 {
		return next.Position, true
	}
	fraction := (millis - float64(previous.TimeMillis)) / span
	return previous.Position + (next.Position-previous.Position)*fraction, true
}
