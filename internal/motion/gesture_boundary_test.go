package motion

import (
	"fmt"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// This seed came from an accepted hosted-model edit that could not start:
// a band ending at 100 produced an out-of-bounds authored point.
func TestGestureAnchoredBandsStayInsideExactBounds(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	for _, seed := range []uint32{2947866601, 3236588786, 1, 13} {
		for _, width := range []int{10, 25, 100} {
			for _, tip := range []bool{false, true} {
				t.Run(fmt.Sprintf("seed_%d_width_%d_tip_%t", seed, width, tip), func(t *testing.T) {
					spec := DefaultFlowSpec()
					spec.RangeFloorPercent = 10
					spec.MinPercent, spec.MaxPercent, spec.SpeedPercent, spec.Seed = 0, width, 25, seed
					g := DefaultGestureSpec()
					g.FocusPercent, g.FocusWidthPercent, g.FocusMixPercent, g.FocusRoamPercent = 0, width, 100, 0
					g.FasterDirection, g.ContrastPercent = "tip", 50
					if tip {
						spec.MinPercent, spec.MaxPercent, g.FocusPercent = 100-width, 100, 100
					}
					spec.Gesture = &g
					for _, leg := range gestureLegs(spec) {
						for _, x := range []float64{leg.from, leg.to} {
							if x < float64(spec.MinPercent) || x > float64(spec.MaxPercent) {
								t.Fatalf("authored endpoint %.17g escaped band [%d,%d]", x, spec.MinPercent, spec.MaxPercent)
							}
						}
					}
					if _, err := FlowTarget(spec, settings); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}
