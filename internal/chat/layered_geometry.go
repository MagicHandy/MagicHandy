package chat

import (
	"errors"
	"slices"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

var layeredGeometries = []string{"alternate_ends", "full_and_tip", "full_and_base", "tip_anchor", "base_anchor", "centered", "wander"}

// Geometry operations are explicit model-selected edits of coupled controls,
// not text inference or a fallback catalog. Independent pace layers survive.
func applyLayeredGeometry(s *motion.FlowSpec, geometry string) error {
	if geometry == "" {
		return nil
	}
	if !slices.Contains(layeredGeometries, geometry) {
		return errors.New("unknown Layered geometry")
	}
	remove := func(axis string) {
		s.Layers = slices.DeleteFunc(s.Layers, func(l motion.FlowLayer) bool { return l.Axis == axis })
	}
	set := func(layer motion.FlowLayer) { remove(layer.Axis); s.Layers = append(s.Layers, layer) }
	width := s.MaxPercent - s.MinPercent
	switch geometry {
	case "alternate_ends":
		if width < 20 {
			return errors.New("alternating ends needs an outer band at least 20 wide")
		}
		s.RangeFloorPercent = max(10, min(15, width/3))
		s.RangeCeilingPercent = min(30, width/2)
		remove("range")
		set(motion.FlowLayer{Axis: "center", AmountPercent: 100, PeriodCycles: 8, Shape: "alternate"})
	case "full_and_tip", "full_and_base":
		if width < 20 {
			return errors.New("full/local alternation needs an outer band at least 20 wide")
		}
		s.AnchorPercent = 100
		if geometry == "full_and_base" {
			s.AnchorPercent = 0
		}
		s.RangeFloorPercent = min(20, width/2)
		s.RangeCeilingPercent = width
		remove("center")
		set(motion.FlowLayer{Axis: "range", AmountPercent: 100, PeriodCycles: 8, Shape: "alternate"})
	case "tip_anchor", "base_anchor", "centered":
		s.AnchorPercent = 50
		if geometry == "tip_anchor" {
			s.AnchorPercent = 100
		}
		if geometry == "base_anchor" {
			s.AnchorPercent = 0
		}
		remove("center")
	case "wander":
		s.RangeFloorPercent = min(20, width)
		s.RangeCeilingPercent = width
		s.VariationMode = "drift"
		remove("range")
		set(motion.FlowLayer{Axis: "center", AmountPercent: 35, PeriodCycles: 14, PhasePercent: 17, Shape: "drift"})
	}
	return nil
}

func validateLayeredGeometry(s motion.FlowSpec, geometry string) error {
	axis := func(name string) motion.FlowLayer {
		for _, l := range s.Layers {
			if l.Axis == name {
				return l
			}
		}
		return motion.FlowLayer{}
	}
	full := func(l motion.FlowLayer) bool { return l.AmountPercent == 100 && l.Shape == "alternate" }
	valid := true
	switch geometry {
	case "alternate_ends":
		valid = s.RangeFloorPercent <= s.RangeCeilingPercent && s.RangeCeilingPercent <= ((s.MaxPercent-s.MinPercent)/2) && full(axis("center")) && axis("range").Axis == ""
	case "full_and_tip", "full_and_base":
		anchor := 100
		if geometry == "full_and_base" {
			anchor = 0
		}
		valid = s.AnchorPercent == anchor && s.RangeCeilingPercent == s.MaxPercent-s.MinPercent && s.RangeFloorPercent < s.RangeCeilingPercent && full(axis("range")) && axis("center").Axis == ""
	case "tip_anchor", "base_anchor", "centered":
		anchor := 50
		if geometry == "tip_anchor" {
			anchor = 100
		}
		if geometry == "base_anchor" {
			anchor = 0
		}
		valid = s.AnchorPercent == anchor && axis("center").Axis == ""
	}
	if !valid {
		return errors.New("edits conflict with the selected geometry")
	}
	return nil
}

func applyLayeredGeometryEdit(next *motion.FlowSpec, edit LayeredEdit) error {
	anchor, variation := next.AnchorPercent, next.VariationMode
	if err := applyLayeredGeometry(next, edit.Geometry); err != nil {
		return err
	}
	if ((edit.Controls["anchor_percent"] != nil || edit.ChangeBy["anchor_percent"] != nil) && anchor != next.AnchorPercent) || (edit.Controls["variation_mode"] != nil && variation != next.VariationMode) {
		return errors.New("controls conflict with the selected geometry")
	}

	return nil
}
