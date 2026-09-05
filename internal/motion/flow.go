package motion

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// FlowSpec describes a continuous carrier and slow modulation in cycle space.
// It does not use DynamicDefinition, span profiles, authored reversal routes,
// or the Creative interval fitter. All values are semantic, never wire points.
type FlowSpec struct {
	Gesture              *GestureSpec `json:"gesture,omitempty"`
	MinPercent           int          `json:"min_percent"`
	MaxPercent           int          `json:"max_percent"`
	SpeedPercent         int          `json:"speed_percent"`
	RangeFloorPercent    int          `json:"range_floor_percent"`
	RangeCeilingPercent  int          `json:"range_ceiling_percent,omitempty"`
	AnchorPercent        int          `json:"anchor_percent"`
	MemoryCycles         int          `json:"memory_cycles"`
	PaceVariationPercent int          `json:"pace_variation_percent"`
	VariationMode        string       `json:"variation_mode,omitempty"`
	TurnSoftnessPercent  int          `json:"turn_softness_percent,omitempty"`
	CadenceHoldPercent   int          `json:"cadence_hold_percent,omitempty"`
	Seed                 uint32       `json:"seed"`
	LoopCycles           int          `json:"loop_cycles,omitempty"`
	Steps                []FlowStep   `json:"steps,omitempty"`
	Layers               []FlowLayer  `json:"layers,omitempty"`
}

// FlowStep is a section of one continuous score, not a separate motion run.
type FlowStep struct {
	MinPercent   int `json:"min_percent"`
	MaxPercent   int `json:"max_percent"`
	SpeedPercent int `json:"speed_percent"`
	Cycles       int `json:"cycles"`
}

// FlowLayer modulates an axis of the carrier; positions are never added or
// clipped. Convex envelopes keep all combinations inside the requested band.
type FlowLayer struct {
	Axis          string `json:"axis"`
	AmountPercent int    `json:"amount_percent"`
	PeriodCycles  int    `json:"period_cycles"`
	PhasePercent  int    `json:"phase_percent"`
	Shape         string `json:"shape,omitempty"`
}

// DefaultFlowSpec supplies a reproducible starting comparison.
func DefaultFlowSpec() FlowSpec {
	return FlowSpec{MinPercent: 5, MaxPercent: 95, SpeedPercent: 25,
		RangeFloorPercent: 25, AnchorPercent: 0, MemoryCycles: 8,
		PaceVariationPercent: 10, Seed: 17}
}

// Validate rejects ambiguous or out-of-bounds scores rather than silently
// clipping model output. Physical speed limits remain backend authoritative.
func (s FlowSpec) Validate(settings config.MotionSettings) error {
	if err := s.validateGesture(); err != nil {
		return err
	}
	if err := s.validateControls(settings); err != nil {
		return err
	}
	if err := s.validateSteps(settings); err != nil {
		return err
	}
	return s.validateLayers()
}

func (s FlowSpec) validateControls(settings config.MotionSettings) error {
	if s.MinPercent < 0 || s.MinPercent > 90 || s.MaxPercent < 10 || s.MaxPercent > 100 || s.MaxPercent-s.MinPercent < 10 ||
		s.RangeFloorPercent < 10 || s.RangeFloorPercent > s.MaxPercent-s.MinPercent ||
		s.AnchorPercent < 0 || s.AnchorPercent > 100 || s.MemoryCycles < 2 || s.MemoryCycles > 32 ||
		s.PaceVariationPercent < 0 || s.PaceVariationPercent > 40 || s.Seed == 0 ||
		len(s.Steps) > 4 || len(s.Layers) > 3 {
		return errors.New("flow controls exceed their supported bounds")
	}
	settings = normalizeMotionSettings(settings)
	if s.SpeedPercent < settings.SpeedMinPercent || s.SpeedPercent > settings.SpeedMaxPercent {
		return errors.New("flow speed must stay inside the saved speed limits")
	}
	return s.validateOptionalControls()
}

func (s FlowSpec) validateOptionalControls() error {
	if err := s.validateExperiments(); err != nil {
		return err
	}
	if s.RangeCeilingPercent != 0 && (s.RangeCeilingPercent < s.RangeFloorPercent || s.RangeCeilingPercent > s.MaxPercent-s.MinPercent) {
		return errors.New("flow range ceiling must be zero for the outer width or between the floor and outer width")
	}
	if s.LoopCycles != 0 && (s.LoopCycles < 4 || s.LoopCycles > 64) {
		return errors.New("flow loop cycles must be zero for the default or 4–64")
	}
	return nil
}

func (s FlowSpec) validateSteps(settings config.MotionSettings) error {
	settings = normalizeMotionSettings(settings)
	for _, step := range s.Steps {
		if step.MinPercent < 0 || step.MaxPercent > 100 || step.MaxPercent-step.MinPercent < 10 ||
			step.Cycles < 2 || step.Cycles > 12 || step.SpeedPercent < settings.SpeedMinPercent || step.SpeedPercent > settings.SpeedMaxPercent {
			return errors.New("each flow step needs a valid band, 2–12 cycles and a speed inside saved limits")
		}
	}
	return nil
}

func (s FlowSpec) validateLayers() error {
	seen := make(map[string]bool)
	for _, layer := range s.Layers {
		if (layer.Axis != "range" && layer.Axis != "center" && layer.Axis != "pace") || seen[layer.Axis] ||
			layer.AmountPercent < 0 || layer.AmountPercent > 100 || layer.PeriodCycles < 2 || layer.PeriodCycles > 32 ||
			layer.PhasePercent < 0 || layer.PhasePercent > 100 ||
			(layer.Shape != "" && layer.Shape != "wave" && layer.Shape != "drift" && layer.Shape != "alternate") {
			return errors.New("flow layers require distinct range/center/pace axes and bounded modulation")
		}
		seen[layer.Axis] = true
	}
	return nil
}

const flowAccelerationBudget = 2400.0
const flowJerkBudget = 24000.0

// FlowTarget compiles the score once into immutable kinematic content. The
// ordinary engine owns playback, live limit changes, handoffs and Stop.
func FlowTarget(spec FlowSpec, settings config.MotionSettings) (MotionTarget, error) {
	if err := spec.Validate(settings); err != nil {
		return MotionTarget{}, err
	}
	var curve Curve
	var err error
	name := "Continuous flow"
	if spec.Gesture != nil {
		curve, err = compileGestureCurve(spec, settings.HandyModel)
		name = "Creative v2"
	} else {
		curve, err = compileFlowCurve(spec, settings.HandyModel)
	}
	if err != nil {
		return MotionTarget{}, err
	}
	encoded, _ := json.Marshal(spec)
	hash := fnv.New64a()
	_, _ = hash.Write(encoded)
	id := fmt.Sprintf("flow-%x", hash.Sum64())
	peakSpeed := spec.SpeedPercent
	for _, step := range spec.Steps {
		peakSpeed = max(peakSpeed, step.SpeedPercent)
	}
	content := &preparedMotion{id: id, name: name, curve: curve,
		referenceRate: referenceTravelRateForSpeed(peakSpeed, settings.HandyModel),
		acceleration:  flowAccelerationBudget, jerk: flowJerkBudget}
	if spec.Gesture != nil {
		// The new grammar uses Creative's existing runtime envelope. Historical
		// flow comparisons keep their quieter authoring budget unchanged.
		content.acceleration, content.jerk = runtimeMaxAccelerationPercentPerSecond2, runtimeMaxJerkPercentPerSecond3
	}
	return MotionTarget{Label: name, Source: TargetSourceMotionLab,
		SpeedPercent: peakSpeed, Flow: CloneFlowSpec(&spec), prepared: content}, nil
}

func (s FlowSpec) cycleCount() float64 {
	if len(s.Steps) == 0 {
		if s.LoopCycles > 0 {
			return float64(s.LoopCycles)
		}
		return 64
	}
	cycles := 0
	for _, step := range s.Steps {
		cycles += step.Cycles
	}
	return float64(cycles)
}

func flowSeptic(u float64) float64 {
	u = clampFloat(u, 0, 1)
	return u * u * u * u * (35 + u*(-84+u*(70-20*u)))
}

func (s FlowSpec) band(u float64) (float64, float64, float64, float64) {
	if len(s.Steps) == 0 {
		return float64(s.MinPercent), float64(s.MaxPercent), float64(s.SpeedPercent), float64(s.RangeFloorPercent)
	}
	u = positiveModulo(u, s.cycleCount())
	start := 0.0
	for index, step := range s.Steps {
		end := start + float64(step.Cycles)
		if u < end {
			previous := s.Steps[(index+len(s.Steps)-1)%len(s.Steps)]
			blend := flowSeptic((u - start) / 0.8)
			mix := func(a, b int) float64 { return float64(a) + (float64(b)-float64(a))*blend }
			floor := mix(min(s.RangeFloorPercent, previous.MaxPercent-previous.MinPercent), min(s.RangeFloorPercent, step.MaxPercent-step.MinPercent))
			return mix(previous.MinPercent, step.MinPercent), mix(previous.MaxPercent, step.MaxPercent), mix(previous.SpeedPercent, step.SpeedPercent), floor
		}
		start = end
	}
	return float64(s.MinPercent), float64(s.MaxPercent), float64(s.SpeedPercent), float64(s.RangeFloorPercent)
}

func positiveModulo(value, period float64) float64 {
	return value - period*math.Floor(value/period)
}

func flowWave(u, cycles, memory, phase float64) float64 {
	waves := math.Max(1, math.Round(cycles/memory))
	return math.Cos(2 * math.Pi * (u*waves/cycles + phase))
}

func (s FlowSpec) field(u float64, salt uint32) float64 {
	if s.VariationMode == "drift" {
		return s.driftField(u, salt)
	}
	seed := s.Seed ^ salt
	phase := func() float64 {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		return float64(seed) / float64(math.MaxUint32)
	}
	cycles, memory := s.cycleCount(), float64(s.MemoryCycles)
	return 0.5 + 0.5*(0.5*flowWave(u, cycles, memory, phase())+
		0.3*flowWave(u, cycles, memory*1.7, phase())+0.2*flowWave(u, cycles, memory/1.6, phase()))
}

func (s FlowSpec) signal(u float64, handyModel string) (position, millisPerCycle float64) {
	lo, hi, speed, floor := s.band(u)
	width := hi - lo
	ceiling := width
	if s.RangeCeilingPercent > 0 {
		ceiling = clampFloat(float64(s.RangeCeilingPercent), floor, width)
	}
	span := floor + (ceiling-floor)*s.field(u, 0x9e3779b9)
	anchor := float64(s.AnchorPercent) / 100
	paceVariation := float64(s.PaceVariationPercent) / 100
	pace := 1 - paceVariation + paceVariation*s.field(u, 0x85ebca6b)
	for _, layer := range s.Layers {
		amount := float64(layer.AmountPercent) / 100
		wave := s.layerEnvelope(u, layer)
		switch layer.Axis {
		case "range":
			if layer.Shape == "" {
				span = floor + (span-floor)*(1-amount+amount*wave)
			} else {
				span = (1-amount)*span + amount*(floor+(ceiling-floor)*wave)
			}
		case "center":
			anchor = (1-amount)*anchor + amount*wave
		case "pace":
			pace *= 1 - 0.5*amount + 0.5*amount*wave
		}
	}
	position = lo + anchor*(width-span) + span*s.carrier(u)
	// The clock compensates for span, so a short stroke does not automatically
	// become a slow stroke. Section speeds interpolate continuously as rates.
	lowerSpeed := int(math.Floor(speed))
	rate := referenceTravelRateForSpeed(lowerSpeed, handyModel)
	rate += (referenceTravelRateForSpeed(min(lowerSpeed+1, 100), handyModel) - rate) * (speed - float64(lowerSpeed))
	clockSpan := span + float64(s.CadenceHoldPercent)/100*(ceiling-span)
	desired := 2 * clockSpan / (rate * pace)
	accelerationTime := math.Pi * math.Sqrt(2*span/flowAccelerationBudget)
	jerkTime := math.Pi * math.Cbrt(4*span/flowJerkBudget)
	if s.TurnSoftnessPercent > 0 {
		softness := float64(s.TurnSoftnessPercent) / 100
		// Convex derivative bounds of cosine and the septic half-stroke.
		// Exact compiled extrema still gate the shared plan below this clock.
		accelerationTime *= math.Sqrt(1 + softness*((67.2/math.Sqrt(5))/(2*math.Pi*math.Pi)-1))
		jerkTime *= math.Cbrt(1 + softness*(420/(4*math.Pi*math.Pi*math.Pi)-1))
	}
	// Smooth local clock limiting anticipates tight strokes without imposing
	// their time requirement on every broad stroke. Exact extrema still gate playback.
	period := math.Pow(math.Pow(desired, 8)+math.Pow(accelerationTime, 8)+math.Pow(jerkTime, 8), 1.0/8)
	return position, 1000 * period
}
