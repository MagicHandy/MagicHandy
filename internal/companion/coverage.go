package companion

import "time"

// Coverage states. A linked session treats starving and starved alike as
// unhealthy, so an owner must declare enough buffered lead that a normal run
// never reports either.
const (
	StateCovered  = "playing"
	StateStarving = "starving"
	StateStarved  = "starved"
)

// Coverage turns a frame that ran out into a reported starvation: output is
// silent at once, and after the grace period the owner stops and reports
// starved, the same stop-and-report rule the Intiface pacer follows.
type Coverage struct {
	grace          time.Duration
	uncoveredSince time.Time
}

// NewCoverage returns a tracker that reports starved after grace.
func NewCoverage(grace time.Duration) Coverage {
	return Coverage{grace: grace}
}

// Observe records whether the frame covered this tick and returns the state.
func (c *Coverage) Observe(now time.Time, covered bool) string {
	if covered {
		c.uncoveredSince = time.Time{}
		return StateCovered
	}
	if c.uncoveredSince.IsZero() {
		c.uncoveredSince = now
	}
	if now.Sub(c.uncoveredSince) >= c.grace {
		return StateStarved
	}
	return StateStarving
}

// Reset forgets any uncovered interval, for a new or resumed stream.
func (c *Coverage) Reset() {
	c.uncoveredSince = time.Time{}
}
