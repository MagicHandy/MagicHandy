package motion

import (
	"errors"
	"fmt"
	"math"
)

// Tempo modes (ADR 0071). Tempo is the native channel of a stroker that makes
// its own strokes, such as the VacuGlide: how fast it strokes, not where.
const (
	// TempoModeFollow derives tempo from the moving positions.
	TempoModeFollow = "follow"
	// TempoModeSteady holds one level.
	TempoModeSteady = "steady"
	// TempoModeRamp moves from the current level to level_percent.
	TempoModeRamp = "ramp"
	// TempoModeWave swells between min_percent and max_percent.
	TempoModeWave = "wave"
	// TempoModePulse lifts to max_percent in rounded bursts.
	TempoModePulse = "pulse"
	// TempoModePause stops the motor.
	TempoModePause = "pause"
	// TempoModeSequence plays sections in order.
	TempoModeSequence = "sequence"
	// TempoModeRoutine plays a built-in routine.
	TempoModeRoutine = "routine"
)

const (
	maxTempoSections       = 12
	minTempoPeriodSeconds  = 2.0
	maxTempoPeriodSeconds  = 120.0
	minTempoRampSeconds    = 1.0
	maxTempoRampSeconds    = 600.0
	maxTempoSectionSeconds = 1800.0
	minTempoLoopSeconds    = 4.0
	// A new section or envelope blends in from the level it replaces, so a
	// sequence never jumps the motor. Ramps start from it by definition and a
	// pause stops at once.
	tempoBlendMillis = 1000.0
)

// TempoSection is one stretch of tempo. Levels are percentages of the
// person's Tempo range: 0 stops the motor and 1..100 span the range. Seconds
// is how long the section plays inside a sequence.
type TempoSection struct {
	Mode          string  `json:"mode"`
	LevelPercent  int     `json:"level_percent,omitempty"`
	MinPercent    int     `json:"min_percent,omitempty"`
	MaxPercent    int     `json:"max_percent,omitempty"`
	PeriodSeconds float64 `json:"period_seconds,omitempty"`
	Seconds       float64 `json:"seconds,omitempty"`
}

// TempoSpec is semantic tempo, never a motor speed or a change to the
// person's Tempo range. A single mode uses the section fields directly; a
// sequence lists sections; a routine names a built-in one, varied by Seed.
type TempoSpec struct {
	Mode          string         `json:"mode"`
	LevelPercent  int            `json:"level_percent,omitempty"`
	MinPercent    int            `json:"min_percent,omitempty"`
	MaxPercent    int            `json:"max_percent,omitempty"`
	PeriodSeconds float64        `json:"period_seconds,omitempty"`
	Sections      []TempoSection `json:"sections,omitempty"`
	Loop          bool           `json:"loop,omitempty"`
	Routine       string         `json:"routine,omitempty"`
	Seed          uint64         `json:"seed,omitempty"`
}

// Clone returns an independent copy.
func (s TempoSpec) Clone() TempoSpec {
	s.Sections = append([]TempoSection(nil), s.Sections...)
	return s
}

// CloneTempoSpec copies an optional tempo, including its sequence sections.
func CloneTempoSpec(spec *TempoSpec) *TempoSpec {
	if spec == nil {
		return nil
	}
	cloned := spec.Clone()
	return &cloned
}

// tempoScalars is the comparable part of a spec.
type tempoScalars struct {
	section TempoSection
	loop    bool
	routine string
	seed    uint64
}

func (s TempoSpec) scalars() tempoScalars {
	return tempoScalars{section: s.single(), loop: s.Loop, routine: s.Routine, seed: s.Seed}
}

// Equal reports whether two specs ask for the same tempo.
func (s TempoSpec) Equal(other TempoSpec) bool {
	if s.scalars() != other.scalars() || len(s.Sections) != len(other.Sections) {
		return false
	}
	for index := range s.Sections {
		if s.Sections[index] != other.Sections[index] {
			return false
		}
	}
	return true
}

func (s TempoSpec) single() TempoSection {
	return TempoSection{Mode: s.Mode, LevelPercent: s.LevelPercent, MinPercent: s.MinPercent, MaxPercent: s.MaxPercent, PeriodSeconds: s.PeriodSeconds}
}

// Validate rejects malformed tempo without reshaping it.
func (s TempoSpec) Validate() error {
	switch s.Mode {
	case TempoModeFollow:
		if s.scalars() != (tempoScalars{section: TempoSection{Mode: TempoModeFollow}}) || len(s.Sections) != 0 {
			return errors.New("tempo follow takes no levels, sections or routine")
		}
		return nil
	case TempoModeSequence:
		if s.single() != (TempoSection{Mode: TempoModeSequence}) || s.Routine != "" {
			return errors.New("a tempo sequence carries its levels in sections")
		}
		return validateTempoSections(s.Sections, s.Loop)
	case TempoModeRoutine:
		if _, ok := TempoRoutineByID(s.Routine); !ok {
			return fmt.Errorf("unknown tempo routine %q", s.Routine)
		}
		if s.single() != (TempoSection{Mode: TempoModeRoutine}) || len(s.Sections) != 0 || s.Loop {
			return errors.New("a tempo routine takes only its routine id")
		}
		return nil
	}
	if len(s.Sections) != 0 || s.Loop || s.Routine != "" {
		return errors.New("only a tempo sequence takes sections or loop, and only a routine takes routine")
	}
	return validateTempoSection(s.single(), false)
}

func validateTempoSections(sections []TempoSection, loop bool) error {
	if len(sections) == 0 || len(sections) > maxTempoSections {
		return fmt.Errorf("a tempo sequence needs 1 to %d sections", maxTempoSections)
	}
	total := 0.0
	for index, section := range sections {
		if section.Mode == TempoModeFollow || section.Mode == TempoModeSequence || section.Mode == TempoModeRoutine {
			return fmt.Errorf("section %d: mode must be steady, ramp, wave, pulse or pause", index+1)
		}
		if err := validateTempoSection(section, true); err != nil {
			return fmt.Errorf("section %d: %w", index+1, err)
		}
		holds := index == len(sections)-1 && !loop
		if section.Seconds == 0 && !holds {
			return fmt.Errorf("section %d: seconds must be 1..%g except for a last section that holds", index+1, maxTempoSectionSeconds)
		}
		total += section.Seconds
	}
	if loop && total < minTempoLoopSeconds {
		return fmt.Errorf("a looping tempo sequence must last at least %g seconds", minTempoLoopSeconds)
	}
	return nil
}

func validateTempoSection(section TempoSection, inSequence bool) error {
	if !finite(section.PeriodSeconds) || !finite(section.Seconds) {
		return errors.New("tempo periods and seconds must be finite")
	}
	if inSequence {
		if section.Seconds != 0 && (section.Seconds < 1 || section.Seconds > maxTempoSectionSeconds) {
			return fmt.Errorf("seconds must be 1..%g", maxTempoSectionSeconds)
		}
	} else if section.Seconds != 0 {
		return errors.New("only sequence sections take seconds")
	}
	validate, ok := tempoSectionValidators[section.Mode]
	if !ok {
		return errors.New("tempo mode must be follow, steady, ramp, wave, pulse, pause, sequence or routine")
	}
	return validate(section)
}

var tempoSectionValidators = map[string]func(TempoSection) error{
	TempoModeSteady: func(section TempoSection) error {
		if section.LevelPercent < 1 || section.LevelPercent > 100 || !levelOnly(section) || section.PeriodSeconds != 0 {
			return errors.New("steady tempo takes level_percent 1..100 only; use pause to stop")
		}
		return nil
	},
	TempoModeRamp: func(section TempoSection) error {
		if section.LevelPercent < 0 || section.LevelPercent > 100 || !levelOnly(section) {
			return errors.New("a tempo ramp takes level_percent 0..100 and period_seconds")
		}
		if section.PeriodSeconds < minTempoRampSeconds || section.PeriodSeconds > maxTempoRampSeconds {
			return fmt.Errorf("a tempo ramp's period_seconds must be %g..%g", minTempoRampSeconds, maxTempoRampSeconds)
		}
		return nil
	},
	TempoModeWave:  validateTempoSwing,
	TempoModePulse: validateTempoSwing,
	TempoModePause: func(section TempoSection) error {
		if section.LevelPercent != 0 || !levelOnly(section) || section.PeriodSeconds != 0 {
			return errors.New("a tempo pause takes no levels")
		}
		return nil
	},
}

func validateTempoSwing(section TempoSection) error {
	if section.LevelPercent != 0 || section.MinPercent < 0 || section.MaxPercent > 100 || section.MinPercent >= section.MaxPercent {
		return errors.New("tempo wave and pulse take 0 <= min_percent < max_percent <= 100")
	}
	if section.PeriodSeconds < minTempoPeriodSeconds || section.PeriodSeconds > maxTempoPeriodSeconds {
		return fmt.Errorf("tempo wave and pulse period_seconds must be %g..%g", minTempoPeriodSeconds, maxTempoPeriodSeconds)
	}
	return nil
}

func levelOnly(section TempoSection) bool { return section.MinPercent == 0 && section.MaxPercent == 0 }

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

// sections resolves a spec to what plays. Follow has none.
func (s TempoSpec) sections() []TempoSection {
	switch s.Mode {
	case TempoModeFollow:
		return nil
	case TempoModeSequence:
		return append([]TempoSection(nil), s.Sections...)
	case TempoModeRoutine:
		routine, _ := TempoRoutineByID(s.Routine)
		return routine.sections(s.Seed)
	default:
		return []TempoSection{s.single()}
	}
}

func (s TempoSpec) loops() bool {
	if s.Mode == TempoModeRoutine {
		routine, _ := TempoRoutineByID(s.Routine)
		return routine.Loop
	}
	return s.Mode == TempoModeSequence && s.Loop
}

// tempoPlan samples one admitted envelope on the stream clock. It is
// immutable once built; a change builds a new plan from the current level.
type tempoPlan struct {
	spec       TempoSpec
	sections   []TempoSection
	durations  []int64
	pass       int64 // one pass of a looping plan, in milliseconds
	loop       bool
	fromMillis int64
	firstStart []float64 // each section's starting level on the first pass
	loopStart  []float64 // and on every later pass
	loopOrigin float64
	loopCarry  float64
	loopOffset float64
	resumeFade bool
	revision   uint64
}

func newTempoPlan(spec TempoSpec, fromMillis int64, startLevel float64, revision uint64) *tempoPlan {
	plan := &tempoPlan{spec: spec.Clone(), sections: spec.sections(), loop: spec.loops(), fromMillis: fromMillis, revision: revision}
	for _, section := range plan.sections {
		duration := int64(math.Round(section.Seconds * 1000))
		plan.durations = append(plan.durations, duration)
		plan.pass += duration
	}
	if !plan.loop || plan.pass <= 0 {
		plan.loop = false
	}
	plan.firstStart = plan.startLevels(clampTempo(startLevel))
	if plan.loop {
		last := len(plan.sections) - 1
		end := tempoSectionLevel(plan.sections[last], plan.firstStart[last], float64(plan.durations[last]))
		plan.loopStart = plan.startLevels(end)
		// Every section is affine in its entry level. Carry that entry across
		// passes rather than reusing pass one's endpoint: a ramp shorter than
		// its requested duration may still be approaching its destination.
		plan.loopOrigin = clampTempo(startLevel)
		plan.loopOffset = plan.passEnd(0)
		plan.loopCarry = math.Max(0, math.Min(1, (plan.passEnd(100)-plan.loopOffset)/100))
	}
	return plan
}

func (p *tempoPlan) follow() bool { return p.spec.Mode == TempoModeFollow }

func (p *tempoPlan) passEnd(origin float64) float64 {
	starts := p.startLevels(origin)
	last := len(p.sections) - 1
	return tempoSectionLevel(p.sections[last], starts[last], float64(p.durations[last]))
}

func (p *tempoPlan) startLevels(origin float64) []float64 {
	levels := make([]float64, len(p.sections))
	level := origin
	for index, section := range p.sections {
		levels[index] = level
		if index < len(p.sections)-1 {
			level = tempoSectionLevel(section, level, float64(p.durations[index]))
		}
	}
	return levels
}

// LevelAt samples the envelope at stream time at, 0..100.
func (p *tempoPlan) LevelAt(at int64) float64 {
	level := p.levelAt(at)
	if p.resumeFade && at < int64(tempoBlendMillis) {
		level *= smoothVibration(clampVibration(float64(at) / tempoBlendMillis))
	}
	return level
}

func (p *tempoPlan) levelAt(at int64) float64 {
	if p.follow() || len(p.sections) == 0 {
		return 0
	}
	elapsed := max(int64(0), at-p.fromMillis)
	starts := p.firstStart
	if p.loop && elapsed >= p.pass {
		passes := elapsed / p.pass
		elapsed %= p.pass
		starts = p.loopStart
		if p.loopCarry > 0 {
			power := math.Pow(p.loopCarry, float64(passes))
			origin := p.loopOrigin + float64(passes)*p.loopOffset
			if p.loopCarry < 1 {
				origin = p.loopOrigin*power + p.loopOffset*(1-power)/(1-p.loopCarry)
			}
			starts = p.startLevels(clampTempo(origin))
		}
	}
	for index, section := range p.sections {
		duration := p.durations[index]
		if duration <= 0 || elapsed < duration || index == len(p.sections)-1 {
			return clampTempo(tempoSectionLevel(section, starts[index], float64(elapsed)))
		}
		elapsed -= duration
	}
	return 0
}

// tempoSectionLevel is a section's level elapsed milliseconds after it began
// from origin.
func tempoSectionLevel(section TempoSection, origin, elapsed float64) float64 {
	var level float64
	switch section.Mode {
	case TempoModePause:
		return 0
	case TempoModeRamp:
		return origin + (float64(section.LevelPercent)-origin)*smoothVibration(clampVibration(elapsed/(section.PeriodSeconds*1000)))
	case TempoModeSteady:
		level = float64(section.LevelPercent)
	case TempoModeWave, TempoModePulse:
		phase := math.Mod(elapsed, section.PeriodSeconds*1000) / (section.PeriodSeconds * 1000)
		x := (1 - math.Cos(2*math.Pi*phase)) / 2
		if section.Mode == TempoModePulse {
			x = smoothVibration(clampVibration((x - 0.25) * 2))
		}
		level = float64(section.MinPercent) + float64(section.MaxPercent-section.MinPercent)*x
	}
	blend := smoothVibration(clampVibration(elapsed / tempoBlendMillis))
	return origin + (level-origin)*blend
}

func clampTempo(level float64) float64 {
	if !finite(level) {
		return 0
	}
	return math.Max(0, math.Min(100, level))
}
