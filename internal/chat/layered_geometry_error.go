package chat

import (
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// LayeredGeometryConflict exposes incompatible JSON fields to correction
// prompts. It validates structured edits; it never classifies user language.
type LayeredGeometryConflict struct {
	Geometry string
	Expected string
	Actual   string
}

func (e *LayeredGeometryConflict) Error() string {
	return fmt.Sprintf("edits conflict with the selected geometry %q: requires %s; received %s. Keep the geometry and remove conflicting refinements, or omit geometry for a custom shape; preserve the user's requested result", e.Geometry, e.Expected, e.Actual)
}

func layeredGeometryConflict(s motion.FlowSpec, geometry string) error {
	width := s.MaxPercent - s.MinPercent
	expected := map[string]string{
		"full_slider":    "outer endpoints 0..100, stroke_width 100..100, and no range/center modulation",
		"full_band":      fmt.Sprintf("stroke_width %d..%d and no range/center modulation", width, width),
		"alternate_ends": fmt.Sprintf("stroke_width.max_percent <= %d, center amount_percent 100 with shape alternate, and no range modulation", width/2),
		"full_and_tip":   fmt.Sprintf("anchor_percent 100, stroke_width.max_percent %d with a smaller minimum, range amount_percent 100 with shape alternate, and no center modulation", width),
		"full_and_base":  fmt.Sprintf("anchor_percent 0, stroke_width.max_percent %d with a smaller minimum, range amount_percent 100 with shape alternate, and no center modulation", width),
		"tip_anchor":     "anchor_percent 100 and no center modulation",
		"base_anchor":    "anchor_percent 0 and no center modulation",
		"centered":       "anchor_percent 50 and no center modulation",
	}[geometry]
	layers := []string{}
	for _, layer := range s.Layers {
		if layer.Axis != "pace" && layer.AmountPercent != 0 {
			layers = append(layers, fmt.Sprintf("%s amount_percent %d shape %s", layer.Axis, layer.AmountPercent, layer.Shape))
		}
	}
	return &LayeredGeometryConflict{Geometry: geometry, Expected: expected,
		Actual: fmt.Sprintf("outer endpoints %d..%d, stroke_width %d..%d, anchor_percent %d, modulation [%s]", s.MinPercent, s.MaxPercent, s.RangeFloorPercent, s.RangeCeilingPercent, s.AnchorPercent, strings.Join(layers, "; "))}
}
