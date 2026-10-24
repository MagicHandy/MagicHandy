package motion

import (
	"math"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Original Handy Cloud auditions favored a sparse representation over extra
// velocity-fit points. The trajectory, clocks and mandatory reversals remain
// unchanged; only optional samples use a larger approximation budget. Keep the
// preference off untested device profiles, immediate owners, imported media and
// transitions. A small stroke must not inherit a full-device error allowance.
func (e *Engine) generatedPositionTolerance(inTransition bool) float64 {
	if !e.preferSparseGeneratedMotion || !e.preservePlanKnots || e.minimumPointIntervalMillis > 0 ||
		e.positionResolutionPercent <= 0 || inTransition || e.plan.Target.Media != nil ||
		!e.plan.Target.continuousCurve() || e.settings.HandyModel != config.HandyModelOriginal {
		return wireApproximationTolerance
	}
	minimumSpan := 100.0
	knots := e.plan.curve.authoredKnots
	found := false
	for i := 1; i < len(knots); i++ {
		span := math.Abs(knots[i].PositionPercent - knots[i-1].PositionPercent)
		if span > 0 {
			minimumSpan = math.Min(minimumSpan, span)
			found = true
		}
	}
	if !found {
		return wireApproximationTolerance
	}
	return math.Max(wireApproximationTolerance, math.Min(3, minimumSpan*.12))
}
