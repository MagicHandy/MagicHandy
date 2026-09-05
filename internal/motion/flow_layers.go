package motion

import (
	"math"
	"slices"
)

// CloneFlowSpec prevents edits or snapshots from aliasing an active score.
func CloneFlowSpec(spec *FlowSpec) *FlowSpec {
	if spec == nil {
		return nil
	}
	cloned := *spec
	if spec.Gesture != nil {
		gesture := *spec.Gesture
		cloned.Gesture = &gesture
	}
	cloned.Steps, cloned.Layers = slices.Clone(spec.Steps), slices.Clone(spec.Layers)
	return &cloned
}

func (s FlowSpec) layerEnvelope(u float64, layer FlowLayer) float64 {
	phase := float64(layer.PhasePercent) / 100
	switch layer.Shape {
	case "drift":
		s.MemoryCycles = layer.PeriodCycles
		salt := uint32(0x27d4eb2d)
		switch layer.Axis {
		case "center":
			salt = 0x165667b1
		case "pace":
			salt = 0xd3a2646c
		}
		return s.driftField(u+phase*s.cycleCount(), salt)
	case "alternate":
		return s.alternatingLayer(u, layer)
	default:
		return 0.5 + 0.5*flowWave(u, s.cycleCount(), float64(layer.PeriodCycles), phase)
	}
}

// Alternation reaches both requested extremes, with seeded unequal dwell and
// transition durations. C3 interpolation preserves smooth derivatives; unlike
// ordered sections, it modulates one continuous carrier and clock.
func (s FlowSpec) alternatingLayer(u float64, layer FlowLayer) float64 {
	waves := max(1, int(math.Round(s.cycleCount()/float64(layer.PeriodCycles))))
	x := positiveModulo(u*float64(waves)/s.cycleCount()+float64(layer.PhasePercent)/100, float64(waves))
	index, phase := uint32(math.Floor(x)), x-math.Floor(x)
	seed := s.Seed ^ 0xa511e9b3
	riseStart := 0.08 + 0.12*flowHashUnit(seed, index*4)
	riseEnd := 0.36 + 0.12*flowHashUnit(seed, index*4+1)
	fallStart := 0.56 + 0.12*flowHashUnit(seed, index*4+2)
	fallEnd := 0.84 + 0.12*flowHashUnit(seed, index*4+3)
	return flowSeptic((phase-riseStart)/(riseEnd-riseStart)) *
		(1 - flowSeptic((phase-fallStart)/(fallEnd-fallStart)))
}
