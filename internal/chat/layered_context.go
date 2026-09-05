package chat

import "github.com/mapledaemon/MagicHandy/internal/motion"

// Project only model-facing controls. Explicit paired widths avoid an omitted
// ceiling (engine zero sentinel) being mistaken for the requested narrow width.
func layeredScoreContext(score motion.FlowSpec) map[string]any {
	ceiling := score.RangeCeilingPercent
	if ceiling == 0 {
		ceiling = score.MaxPercent - score.MinPercent
	}
	return map[string]any{
		"controls":     map[string]any{"min_percent": score.MinPercent, "max_percent": score.MaxPercent, "speed_percent": score.SpeedPercent, "anchor_percent": score.AnchorPercent, "memory_cycles": score.MemoryCycles, "pace_variation_percent": score.PaceVariationPercent, "variation_mode": score.VariationMode},
		"stroke_width": LayeredStrokeWidth{MinPercent: score.RangeFloorPercent, MaxPercent: ceiling},
		"layers":       score.Layers,
	}
}
