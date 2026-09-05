package modes

import (
	"reflect"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// A new seeded realization develops the same requested phrase; it still gets
// applied as fresh content by the ordinary engine target/transition path.
func sameFlowPhrase(left, right *motion.FlowSpec) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := motion.CloneFlowSpec(left), motion.CloneFlowSpec(right)
	l.Seed, r.Seed, l.SpeedPercent, r.SpeedPercent = 1, 1, 1, 1
	return reflect.DeepEqual(l, r)
}
