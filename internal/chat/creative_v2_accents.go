package chat

import (
	"math"
	"math/rand"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// relaxUnchosenAccents lets accents that an autonomous planning turn did not
// choose again fade by a random amount. Omitted edit groups otherwise persist
// forever, so one early choice (a tip-faster sweep, in the live sessions this
// was written against) became the whole session's default even though the
// model rarely revisited it. An accent now persists only while it keeps being
// chosen: the capability stays, the habit does not.
//
// A hold never reaches this point, and a seed-only refresh is the model
// explicitly continuing the current character, so neither fades anything.
// Nothing is strengthened and no bound is widened.
func relaxUnchosenAccents(before, after *motion.FlowSpec) {
	// #nosec G404 -- non-security motion variation inside validated bounds.
	relaxUnchosenAccentsWith(before, after, rand.Float64)
}

func relaxUnchosenAccentsWith(before, after *motion.FlowSpec, random func() float64) {
	if before == nil || after == nil || before.Gesture == nil || after.Gesture == nil || seedOnlyRefresh(*before, *after) {
		return
	}
	b, a := before.Gesture, after.Gesture
	if a.FasterDirection != "even" && a.ContrastPercent > 0 && a.FasterDirection == b.FasterDirection && a.ContrastPercent == b.ContrastPercent {
		a.ContrastPercent = int(math.Round(float64(a.ContrastPercent) * (0.3 + 0.4*random())))
		if a.ContrastPercent < 10 {
			a.ContrastPercent, a.FasterDirection = 0, "even"
		}
	}
	if a.ReboundCount > 0 && a.ReboundCount == b.ReboundCount && a.ReboundDecayPercent == b.ReboundDecayPercent && random() < 0.5 {
		a.ReboundCount--
	}
	if a.InertiaPercent > relaxedInertiaPercent && a.InertiaPercent == b.InertiaPercent {
		a.InertiaPercent = relaxedInertiaPercent + int(math.Round(float64(a.InertiaPercent-relaxedInertiaPercent)*(0.4+0.4*random())))
	}
}

// relaxedInertiaPercent is where strong inertia settles back toward: close to
// the default's gentle late crest, not a flat rest-to-rest stroke.
const relaxedInertiaPercent = 30

// seedOnlyRefresh reports an update whose only change is the realization seed.
func seedOnlyRefresh(before, after motion.FlowSpec) bool {
	return CreativeV2CharacterUnchanged(before, after)
}
