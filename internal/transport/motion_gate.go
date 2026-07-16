package transport

import (
	"errors"
	"sync/atomic"
)

var errMotionCommandInvalidated = errors.New("motion command invalidated by Stop")

// motionCommandGate rejects commands that were admitted before a concurrent
// Stop, including commands waiting behind another transport call.
type motionCommandGate struct {
	generation atomic.Uint64
	stops      atomic.Int64
}

func (g *motionCommandGate) admit() (uint64, error) {
	generation := g.generation.Load()
	if g.stops.Load() > 0 {
		return 0, errMotionCommandInvalidated
	}
	return generation, nil
}

func (g *motionCommandGate) validate(generation uint64) error {
	if g.stops.Load() > 0 || g.generation.Load() != generation {
		return errMotionCommandInvalidated
	}
	return nil
}

func (g *motionCommandGate) beginStop() {
	g.stops.Add(1)
	g.generation.Add(1)
}

func (g *motionCommandGate) endStop() {
	if remaining := g.stops.Add(-1); remaining < 0 {
		panic("motion command gate Stop underflow")
	}
}
