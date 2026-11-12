package motion

import (
	"math"
	"math/rand/v2"
)

// TempoRoutine is a built-in tempo plan for a stroker that makes its own
// strokes (ADR 0071). Its seed varies levels and timing a little, so each run
// differs while keeping the routine's character.
type TempoRoutine struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Loop        bool   `json:"loop"`
	build       func(*tempoRandom) []TempoSection
}

var tempoRoutines = []TempoRoutine{
	{ID: "slow-build", Name: "Slow build", Description: "Starts slow and climbs in swells for about four minutes, then holds near the top.", build: slowBuildRoutine},
	{ID: "swells", Name: "Swells", Description: "Long rises and falls in tempo that never settle.", Loop: true, build: swellsRoutine},
	{ID: "bursts", Name: "Bursts", Description: "A steady middle pace broken by short bursts of fast strokes.", Loop: true, build: burstsRoutine},
	{ID: "stairs", Name: "Stairs", Description: "Steps up through five speeds, then eases back down and climbs again.", Loop: true, build: stairsRoutine},
	{ID: "edge-and-release", Name: "Edge and release", Description: "Builds to a fast pace, holds it, stops, then starts again slowly.", Loop: true, build: edgeRoutine},
	{ID: "wandering", Name: "Wandering", Description: "Drifts between slow and fast at its own unhurried pace.", Loop: true, build: wanderingRoutine},
}

// TempoRoutines lists the built-in routines in display order.
func TempoRoutines() []TempoRoutine {
	return append([]TempoRoutine(nil), tempoRoutines...)
}

// TempoRoutineByID resolves one built-in routine.
func TempoRoutineByID(id string) (TempoRoutine, bool) {
	for _, routine := range tempoRoutines {
		if routine.ID == id {
			return routine, true
		}
	}
	return TempoRoutine{}, false
}

func (r TempoRoutine) sections(seed uint64) []TempoSection {
	if r.build == nil {
		return nil
	}
	return r.build(&tempoRandom{rng: rand.New(rand.NewPCG(seed, 0x7e3a90))}) // #nosec G404 -- seeded musical variation, not security randomness.
}

type tempoRandom struct{ rng *rand.Rand }

// level varies a level by up to spread either way, within 1..100.
func (t *tempoRandom) level(base, spread int) int {
	return max(1, min(100, base+t.rng.IntN(2*spread+1)-spread))
}

// seconds varies a duration by up to fraction either way.
func (t *tempoRandom) seconds(base, fraction float64) float64 {
	return math.Round(base * (1 + fraction*(2*t.rng.Float64()-1)))
}

func steady(level int, seconds float64) TempoSection {
	return TempoSection{Mode: TempoModeSteady, LevelPercent: level, Seconds: seconds}
}

func ramp(level int, period, seconds float64) TempoSection {
	return TempoSection{Mode: TempoModeRamp, LevelPercent: level, PeriodSeconds: period, Seconds: max(seconds, period)}
}

func wave(mode string, low, high int, period, seconds float64) TempoSection {
	low, high = max(0, low), min(100, high)
	if high <= low {
		high = min(100, low+5)
		low = high - 5
	}
	period = math.Max(minTempoPeriodSeconds, math.Min(maxTempoPeriodSeconds, period))
	return TempoSection{Mode: mode, MinPercent: low, MaxPercent: high, PeriodSeconds: period, Seconds: seconds}
}

func slowBuildRoutine(t *tempoRandom) []TempoSection {
	sections := []TempoSection{steady(t.level(18, 4), t.seconds(25, 0.2))}
	for _, step := range []struct{ to, low, high int }{{32, 28, 42}, {52, 46, 60}, {72, 64, 80}} {
		climb := t.seconds(30, 0.2)
		sections = append(sections, ramp(t.level(step.to, 3), climb, climb),
			wave(TempoModeWave, t.level(step.low, 3), t.level(step.high, 3), t.seconds(9, 0.2), t.seconds(40, 0.2)))
	}
	top := t.seconds(25, 0.2)
	return append(sections, ramp(t.level(86, 4), top, 0))
}

func swellsRoutine(t *tempoRandom) []TempoSection {
	return []TempoSection{
		wave(TempoModeWave, t.level(25, 4), t.level(55, 5), t.seconds(14, 0.2), t.seconds(60, 0.2)),
		wave(TempoModeWave, t.level(35, 4), t.level(70, 5), t.seconds(11, 0.2), t.seconds(60, 0.2)),
		wave(TempoModeWave, t.level(30, 4), t.level(62, 5), t.seconds(16, 0.2), t.seconds(45, 0.2)),
	}
}

func burstsRoutine(t *tempoRandom) []TempoSection {
	calm := t.level(32, 5)
	return []TempoSection{
		steady(calm, t.seconds(18, 0.25)),
		wave(TempoModePulse, calm, t.level(80, 5), t.seconds(3, 0.15), t.seconds(12, 0.2)),
		steady(t.level(38, 4), t.seconds(15, 0.25)),
		wave(TempoModePulse, t.level(36, 4), t.level(88, 5), t.seconds(2.5, 0.15), t.seconds(9, 0.2)),
	}
}

func stairsRoutine(t *tempoRandom) []TempoSection {
	var sections []TempoSection
	for _, level := range []int{20, 35, 50, 65, 80} {
		sections = append(sections, steady(t.level(level, 3), t.seconds(18, 0.2)))
	}
	ease := t.seconds(10, 0.2)
	return append(sections, ramp(t.level(20, 3), ease, ease))
}

func edgeRoutine(t *tempoRandom) []TempoSection {
	climb := t.seconds(50, 0.2)
	return []TempoSection{
		ramp(t.level(80, 5), climb, climb),
		steady(t.level(86, 4), t.seconds(12, 0.25)),
		{Mode: TempoModePause, Seconds: t.seconds(8, 0.25)},
		ramp(t.level(25, 4), 4, 6),
		steady(t.level(25, 4), t.seconds(15, 0.25)),
	}
}

func wanderingRoutine(t *tempoRandom) []TempoSection {
	var sections []TempoSection
	for range 3 {
		drift := t.seconds(10, 0.4)
		sections = append(sections, ramp(t.level(48, 27), drift, drift), steady(0, t.seconds(12, 0.35)))
	}
	// A steady section holds the level its ramp reached.
	for index := 1; index < len(sections); index += 2 {
		sections[index].LevelPercent = max(1, sections[index-1].LevelPercent)
	}
	return sections
}
