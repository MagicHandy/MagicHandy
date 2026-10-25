package companion

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// maxTimelinePoints bounds memory for one stream; owners prune points the
// pacer has passed long before this matters.
const maxTimelinePoints = 8192

// ErrTimelineFull rejects an append that would exceed the bound.
var ErrTimelineFull = errors.New("companion timeline is full")

// Point is one timed semantic position in stream milliseconds. Position is
// the shared engine's 0..100 span; 0 is the base end of the stroke.
type Point struct {
	TimeMillis int64
	Position   float64
}

// Timeline holds the current stream's points. It is the owner's read-only
// copy of the shared frame: renderers sample it and never reshape it.
type Timeline struct {
	points []Point
}

// Append adds points in stream order. A point at or before the current end
// replaces the tail from that time, matching how the engine re-plans ahead.
// The append is atomic: an invalid point or a full timeline leaves the
// timeline unchanged.
func (t *Timeline) Append(points []Point) error {
	for _, point := range points {
		if math.IsNaN(point.Position) || point.Position < 0 || point.Position > 100 || point.TimeMillis < 0 {
			return fmt.Errorf("companion point %+v outside the semantic frame", point)
		}
	}
	next := append([]Point(nil), t.points...)
	for _, point := range points {
		if count := len(next); count > 0 && point.TimeMillis <= next[count-1].TimeMillis {
			cut := sort.Search(count, func(index int) bool { return next[index].TimeMillis >= point.TimeMillis })
			next = next[:cut]
		}
		if len(next) >= maxTimelinePoints {
			return ErrTimelineFull
		}
		next = append(next, point)
	}
	t.points = next
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

// Start reports the first retained point. Before it, a new stream has not
// begun; callers must not mistake its preparation time for an exhausted tail.
func (t *Timeline) Start() (int64, bool) {
	if len(t.points) == 0 {
		return 0, false
	}
	return t.points[0].TimeMillis, true
}

// Len reports the number of retained points.
func (t *Timeline) Len() int {
	return len(t.points)
}

// At samples the frame by linear interpolation, which is how the shared
// engine's timed points are played between knots. It reports false outside
// the covered interval: owners output silence there instead of holding the
// last value.
func (t *Timeline) At(millis float64) (float64, bool) {
	count := len(t.points)
	if count == 0 || math.IsNaN(millis) || millis < float64(t.points[0].TimeMillis) || millis > float64(t.points[count-1].TimeMillis) {
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

// Covers reports whether the frame covers every stream time in [from, to].
func (t *Timeline) Covers(from, to float64) bool {
	count := len(t.points)
	return count > 0 && from >= float64(t.points[0].TimeMillis) && to <= float64(t.points[count-1].TimeMillis)
}
