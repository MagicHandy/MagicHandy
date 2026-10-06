package modes

import (
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Freestyle shapes are user-chosen session arcs. They are visible in the mode
// status, bounded inside the user's speed band (energy 1 is the chosen pace,
// never more), and driven only by the backend's active-time clock, which
// pause freezes. They are the same four properties that keep Autopilot's
// buildup from being hidden escalation (docs/autopilot-cadence.md).

// Shape phases reported in the Freestyle status.
const (
	ShapePhaseSteady     = "steady"
	ShapePhaseBuilding   = "building"
	ShapePhaseHolding    = "holding"
	ShapePhaseRising     = "rising"
	ShapePhaseFalling    = "falling"
	ShapePhasePeak       = "peak"
	ShapePhaseBackingOff = "backing_off"
	ShapePhaseTeasing    = "teasing"
	ShapePhaseEasing     = "easing"
	ShapePhaseFinished   = "finished"
)

const (
	buildStartEnergy    = 0.15
	cooldownFinalEnergy = 0.1
	waveLowEnergy       = 0.4
	edgeBuildStart      = 0.25
	edgeTeaseEnergy     = 0.15
	edgeBackOffSeconds  = 8.0
	edgeTeaseOutSeconds = 10.0
)

// freestyleShape is a shape's reading at one moment of active time.
type freestyleShape struct {
	// energy places the pace between the band's floor (0) and the chosen
	// pace (1); tease draws strokes toward short work at the tip.
	energy, tease float64
	phase         string
	// progress runs 0-1 through Slow build and Cooldown.
	progress float64
	// finished reports that Cooldown has run its course.
	finished bool
}

func shapeAt(settings config.FreestyleSettings, seed uint32, active time.Duration) freestyleShape {
	seconds := math.Max(0, active.Seconds())
	minutes := float64(max(1, settings.ShapeMinutes)) * 60
	switch settings.Shape {
	case config.FreestyleShapeBuild:
		u := seconds / minutes
		if u >= 1 {
			return freestyleShape{energy: 1, phase: ShapePhaseHolding, progress: 1}
		}
		return freestyleShape{energy: buildStartEnergy + (1-buildStartEnergy)*shapeEase(u), phase: ShapePhaseBuilding, progress: u}
	case config.FreestyleShapeCooldown:
		u := seconds / minutes
		if u >= 1 {
			return freestyleShape{energy: cooldownFinalEnergy, phase: ShapePhaseFinished, progress: 1, finished: true}
		}
		return freestyleShape{energy: 1 - (1-cooldownFinalEnergy)*shapeEase(u), phase: ShapePhaseEasing, progress: u}
	case config.FreestyleShapeWaves:
		return waveShape(seed, seconds)
	case config.FreestyleShapeEdge:
		return edgeShape(seed, seconds)
	default:
		return freestyleShape{energy: 1, phase: ShapePhaseSteady}
	}
}

// shapeEase rises from 0 to 1 with no slope at either end and a near-linear
// middle, so a long build keeps a steady trend.
func shapeEase(u float64) float64 {
	u = math.Max(0, math.Min(1, u))
	return u - math.Sin(2*math.Pi*u)/(2*math.Pi)
}

// waveShape rises and falls in waves of 100-180 seconds. Each wave's length
// is seeded, so the swell keeps no fixed period. A session starts low.
func waveShape(seed uint32, seconds float64) freestyleShape {
	start := 0.0
	for cycle := uint32(0); ; cycle++ {
		length := 100 + 80*shapeUnit(seed^0x3a7e, cycle)
		if seconds < start+length {
			u := (seconds - start) / length
			phase := ShapePhaseRising
			if u >= 0.5 {
				phase = ShapePhaseFalling
			}
			return freestyleShape{energy: waveLowEnergy + (1-waveLowEnergy)*(1-math.Cos(2*math.Pi*u))/2, phase: phase}
		}
		start += length
	}
}

// edgeShape cycles through a build toward the chosen pace, a short peak, a
// quick back-off into unhurried teasing at the tip, and a teasing stretch,
// then builds again. Every stage length is seeded per cycle.
func edgeShape(seed uint32, seconds float64) freestyleShape {
	start := 0.0
	for cycle := uint32(0); ; cycle++ {
		build := 70 + 70*shapeUnit(seed^0x51b3, cycle)
		peak := 10 + 15*shapeUnit(seed^0x62c4, cycle)
		tease := 25 + 20*shapeUnit(seed^0x73d5, cycle)
		t := seconds - start
		switch {
		case t < build:
			// Later builds rise out of the teasing stretch without a step.
			from := edgeBuildStart
			if cycle > 0 {
				from = edgeTeaseEnergy
			}
			shape := freestyleShape{energy: from + (1-from)*shapeEase(t/build), phase: ShapePhaseBuilding}
			if cycle > 0 {
				shape.tease = math.Max(0, 1-t/edgeTeaseOutSeconds)
			}
			return shape
		case t < build+peak:
			return freestyleShape{energy: 1, phase: ShapePhasePeak}
		case t < build+peak+edgeBackOffSeconds:
			u := smoothUnit((t - build - peak) / edgeBackOffSeconds)
			return freestyleShape{energy: 1 - (1-edgeTeaseEnergy)*u, tease: u, phase: ShapePhaseBackingOff}
		case t < build+peak+edgeBackOffSeconds+tease:
			return freestyleShape{energy: edgeTeaseEnergy, tease: 1, phase: ShapePhaseTeasing}
		}
		start += build + peak + edgeBackOffSeconds + tease
	}
}

func smoothUnit(u float64) float64 {
	u = math.Max(0, math.Min(1, u))
	return u * u * (3 - 2*u)
}

// shapeUnit is a seeded value in [0, 1).
func shapeUnit(seed, index uint32) float64 {
	x := seed ^ (index * 0x9e3779b9)
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return float64(x) / (float64(math.MaxUint32) + 1)
}
