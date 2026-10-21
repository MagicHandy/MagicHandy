package companion

import "time"

// Clock maps a playing stream's milliseconds to local time. The anchor is the
// local time of stream millisecond 0; the zero Clock is stopped.
type Clock struct {
	anchor time.Time
}

// Start anchors a stream that begins at startMillis after preroll from now,
// so the first output is never already late.
func (c *Clock) Start(now time.Time, startMillis int64, preroll time.Duration) {
	c.anchor = now.Add(preroll - time.Duration(startMillis)*time.Millisecond)
}

// StartAt anchors a stream on another clock: stream millisecond 0 is anchor.
func (c *Clock) StartAt(anchor time.Time) {
	c.anchor = anchor
}

// Stop clears the anchor.
func (c *Clock) Stop() {
	c.anchor = time.Time{}
}

// Playing reports whether a stream is anchored.
func (c Clock) Playing() bool {
	return !c.anchor.IsZero()
}

// Anchor returns the local time of stream millisecond 0, or zero.
func (c Clock) Anchor() time.Time {
	return c.anchor
}

// StreamMillis converts a local time to stream milliseconds.
func (c Clock) StreamMillis(at time.Time) float64 {
	return float64(at.Sub(c.anchor)) / float64(time.Millisecond)
}
