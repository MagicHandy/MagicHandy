package chat

import (
	"encoding/json"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func hostedCommandedReach(context MotionContext, capabilities Capabilities) string {
	if !capabilities.AdvancedMotion || context.CommandedSummary == nil {
		return ""
	}
	s := context.CommandedSummary
	facts, _ := json.Marshal(map[string]any{"min_percent": s.PositionMinPercent, "max_percent": s.PositionMaxPercent, "mean_stroke_percent": s.MeanStrokePercent, "turn_coverage": s.Turns})
	return "\nCompiled current reach: " + string(facts) + ". These are actual plan bounds over its whole loop, not merely permission bounds, current position, or measured hardware travel. Do not claim an endpoint was newly reached if it already was; treat reported physical shortfall as unresolved feedback."
}

// Project only model-facing controls. Explicit paired widths avoid an omitted
// ceiling (engine zero sentinel) being mistaken for the requested narrow width.
func layeredScoreContext(score motion.FlowSpec) map[string]any {
	ceiling := score.RangeCeilingPercent
	if ceiling == 0 {
		ceiling = score.MaxPercent - score.MinPercent
	}
	context := map[string]any{
		"controls":     map[string]any{"min_percent": score.MinPercent, "max_percent": score.MaxPercent, "speed_percent": score.SpeedPercent, "anchor_percent": score.AnchorPercent, "memory_cycles": score.MemoryCycles, "pace_variation_percent": score.PaceVariationPercent, "variation_mode": score.VariationMode},
		"stroke_width": LayeredStrokeWidth{MinPercent: score.RangeFloorPercent, MaxPercent: ceiling},
		"layers":       score.Layers,
	}
	if score.Phrase != nil {
		context["phrase"] = score.Phrase
	}
	return context
}
