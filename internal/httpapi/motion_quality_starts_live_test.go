//go:build liveeval && magichandy_labs

package httpapi

import "github.com/mapledaemon/MagicHandy/internal/motion"

// Start each example in a fresh session. A failed start in one conversation
// cannot make every subsequent update fail for want of a running engine.
func qualityIndependentStarts() map[string]qualityTurn {
	band := func(request string, lo, hi, speed int) qualityTurn {
		return qualityTurn{
			"request", request, "start", func(_, a *motion.FlowSpec, r *motion.Review) bool {
				return a != nil && a.SpeedPercent == speed && qualityBounds(r, lo, hi) && r.Summary.MeanStrokePercent >= float64(hi-lo-3)
			},
		}
	}
	return map[string]qualityTurn{
		"start_bounds":          band("Start sweeping from position 16 to position 57, touching both each time, at 35 percent speed.", 16, 57, 35),
		"start_reverse_order":   band("Begin at speed 48. The top of each sweep is 84 and the bottom is 39. Cover the entire interval on every pass.", 39, 84, 48),
		"start_middle":          band("The middle only, please: travel back and forth between 33% and 71% at pace 29. Start now.", 33, 71, 29),
		"start_short_band":      band("Begin a steady rhythm at 45 percent speed. Each pass starts at mark 51 and ends at mark 69, then returns to 51.", 51, 69, 45),
		"start_full":            band("Start full end-to-end sweeps, from 0 to 100, at 37 percent.", 0, 100, 37),
		"start_full_paraphrase": band("Use the whole slider on every pass and start at an overall pace of 52 percent.", 0, 100, 52),
		"start_question":        {"request", "Could the device repeat a sweep between 18 and 62 at 40 percent? Just explain the possibility; leave it idle.", "none", nil},
		"start_future":          {"request", "For a future session I might want long sweeps at 44. For now just acknowledge the idea. Keep the device idle.", "none", nil},
	}
}
