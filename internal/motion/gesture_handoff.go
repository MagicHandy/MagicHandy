package motion

import (
	"math"
	"sort"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// A gesture edit continues nearby in the reach field. A global nearest-point
// search can jump to a different repeated stroke near the start of the phrase
// on every LLM turn, discarding its longer-term progression. Position/direction
// still lead the bounded local search, and the shared transition and sanitizer
// retain authority over the actual handoff.
func chooseGesturePhase(previous MotionPlan, target MotionTarget, settings config.MotionSettings, at int64, position float64, direction int, velocity float64) float64 {
	next := NewMotionPlan("gesture-handoff", target, settings, 0, 0, time.Unix(0, 0))
	if next.compilationError() != nil || len(previous.curve.authoredKnots) < 3 || len(next.curve.authoredKnots) < 3 {
		return chooseNearestPhase(target, settings, position, direction, velocity)
	}
	oldKnots, knots := previous.curve.authoredKnots, next.curve.authoredKnots
	oldTime := previous.PhaseAt(at) * float64(previous.curve.duration)
	oldIndex := max(0, sort.Search(len(oldKnots), func(i int) bool { return float64(oldKnots[i].TimeMillis) > oldTime })-1)
	count := len(knots) - 1
	index := oldIndex * count / (len(oldKnots) - 1)
	best, phase := math.Inf(1), 0.0
	for offset := -4; offset <= 4; offset++ {
		slot := (index + offset + count) % count
		left, right := knots[slot].TimeMillis, knots[slot+1].TimeMillis
		for step := range 65 {
			clock := float64(left) + float64(right-left)*float64(step)/64
			candidateAt := int64(math.Round(clock * float64(next.PeriodMillis) / float64(next.curve.duration)))
			x := next.SampleAt(candidateAt).PositionPercent
			forward := next.SampleAt(candidateAt + 25).PositionPercent
			v := (forward - x) * 20
			score := handoffScore(math.Abs(x-position), direction, curveDirection(forward-x), velocity, v) + 0.75*math.Abs(float64(offset))
			if score < best {
				best, phase = score, float64(candidateAt)/float64(next.PeriodMillis)
			}
		}
	}
	return normalizePhase(phase)
}
