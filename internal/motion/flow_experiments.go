package motion

import (
	"errors"
	"math"
)

// FlowExperiment pairs a frozen semantic score with a comparison prompt.
type FlowExperiment struct {
	ID          string
	Name        string
	Description string
	Spec        FlowSpec
}

// FlowExperiments is the shared roster used by guided tests and visual review.
// It preserves the user's band, layers and sections while isolating new axes.
func FlowExperiments(base FlowSpec) []FlowExperiment {
	base.VariationMode, base.TurnSoftnessPercent, base.CadenceHoldPercent = "", 0, 0
	rows := []FlowExperiment{
		{ID: "reference", Name: "Continuous reference", Description: "Use this round as the reference. Note the rhythm, range changes and reversal feel.", Spec: base},
		{ID: "drift", Name: "Correlated drift", Description: "Check whether the variation feels less repetitive, and note any chatter or unwanted changes in pace.", Spec: base},
		{ID: "soft-turns", Name: "Softer reversals", Description: "Compare the turnarounds and the middle of each stroke. Does lingering help, or feel hesitant?", Spec: base},
		{ID: "steady-beat", Name: "Steadier beat", Description: "Check the beat as stroke length changes. Note whether the more regular timing feels better or simply slower.", Spec: base},
		{ID: "combined", Name: "Combined experiment", Description: "Compare the combined changes with the reference. Describe which qualities improved or became worse.", Spec: base},
	}
	rows[1].Spec.VariationMode = "drift"
	rows[2].Spec.TurnSoftnessPercent = 70
	rows[3].Spec.CadenceHoldPercent = 100
	rows[4].Spec.VariationMode, rows[4].Spec.TurnSoftnessPercent, rows[4].Spec.CadenceHoldPercent = "drift", 70, 100
	return rows
}

// Optional experiments leave existing scores byte- and curve-compatible when
// omitted. They change semantic content only; playback remains engine-owned.
func (s FlowSpec) validateExperiments() error {
	if s.VariationMode != "" && s.VariationMode != "waves" && s.VariationMode != "drift" {
		return errors.New("flow variation mode must be waves or drift")
	}
	if s.TurnSoftnessPercent < 0 || s.TurnSoftnessPercent > 100 || s.CadenceHoldPercent < 0 || s.CadenceHoldPercent > 100 {
		return errors.New("flow turn softness and cadence hold must stay within 0–100")
	}
	return nil
}

func flowHashUnit(seed, index uint32) float64 {
	x := seed ^ (index * 0x9e3779b9)
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return float64(x) / float64(math.MaxUint32)
}

// Periodic value noise with a C3 interpolant. Each envelope has temporal
// correlation and finite support; no new randomness is drawn per sample.
// Two offset scales avoid a single regular swell while preserving the loop.
func (s FlowSpec) driftField(u float64, salt uint32) float64 {
	cycles := s.cycleCount()
	// Validated flow scores bound the loop and memory; this is at most 128.
	knots := uint32(math.Max(2, math.Round(cycles/float64(s.MemoryCycles))))
	field := func(count uint32, seed uint32) float64 {
		phase := flowHashUnit(seed, 0xf00d)
		x := positiveModulo(u*float64(count)/cycles+phase, float64(count))
		index := uint32(math.Floor(x))
		left := flowHashUnit(seed, index)
		right := flowHashUnit(seed, (index+1)%count)
		return left + (right-left)*flowSeptic(x-float64(index))
	}
	return 0.8*field(knots, s.Seed^salt) + 0.2*field(knots*2, s.Seed^salt^0xc2b2ae35)
}

// Blend symmetric cosine travel with zero-velocity/acceleration/jerk endpoints.
// Softness redistributes travel within both half-cycles equally; it never
// compresses only one direction as the retired directional experiment did.
func (s FlowSpec) carrier(u float64) float64 {
	cosine := 0.5 - 0.5*math.Cos(2*math.Pi*u)
	if s.TurnSoftnessPercent == 0 {
		return cosine
	}
	phase := positiveModulo(u, 1)
	septic := flowSeptic(2 * phase)
	if phase >= 0.5 {
		septic = 1 - flowSeptic(2*phase-1)
	}
	return cosine + float64(s.TurnSoftnessPercent)/100*(septic-cosine)
}
