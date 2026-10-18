package chat

import (
	"encoding/json"
	"errors"
)

// These are grammar checks on explicit structured edits, never language rules.
// Missing composition remains compatible with unambiguous earlier responses;
// a selected geometry still cannot be paired with competing manual shape edits.
func normalizeHostedLayeredComposition(edits map[string]json.RawMessage) error {
	geometry, hasGeometry := "", false
	if raw, present := edits["geometry"]; present {
		if json.Unmarshal(raw, &geometry) != nil || geometry == "" {
			return errors.New("geometry must name an operation")
		}
		hasGeometry = true
	}
	if raw, present := edits["composition"]; present {
		var composition string
		if json.Unmarshal(raw, &composition) == nil && composition == "full_slider" {
			if hasGeometry {
				return errors.New("full_slider composition does not also name a geometry")
			}
			geometry, hasGeometry = "full_slider", true
			edits["geometry"] = json.RawMessage(`"full_slider"`)
		} else if (composition != "manual" && composition != "geometry") || (composition == "geometry") != hasGeometry {
			return errors.New("composition must select either manual edits or a named geometry")
		}
	}
	if !hasGeometry {
		return nil
	}
	if _, present := edits["stroke_width"]; present {
		return errors.New("custom stroke width requires manual composition without geometry")
	}
	if err := validateHostedGeometryControls(edits, geometry); err != nil {
		return err
	}
	return validateHostedGeometryLayers(edits)
}

func validateHostedGeometryControls(edits map[string]json.RawMessage, geometry string) error {
	if raw, present := edits["controls"]; present {
		controls, err := uniqueLayeredObject(raw)
		if err != nil {
			return err
		}
		for name := range controls {
			if name == "anchor_percent" || name == "variation_mode" || (geometry == "full_slider" && (name == "min_percent" || name == "max_percent")) {
				return errors.New("named geometry owns its shape controls; use manual composition for custom shape edits")
			}
		}
	}
	return nil
}

func validateHostedGeometryLayers(edits map[string]json.RawMessage) error {
	if raw, present := edits["layers"]; present {
		var layers []json.RawMessage
		if json.Unmarshal(raw, &layers) != nil || layers == nil {
			return errors.New("layers must be an array")
		}
		for _, raw := range layers {
			layer, err := uniqueLayeredObject(raw)
			if err != nil {
				return err
			}
			var axis string
			if json.Unmarshal(layer["axis"], &axis) != nil || axis != "pace" {
				return errors.New("named geometry owns range and center layers; use manual composition to refine them")
			}
		}
	}
	if raw, present := edits["remove_layers"]; present {
		var axes []string
		if json.Unmarshal(raw, &axes) != nil || axes == nil {
			return errors.New("remove_layers must be an array")
		}
		for _, axis := range axes {
			if axis != "pace" {
				return errors.New("named geometry owns range and center layers; use manual composition to refine them")
			}
		}
	}
	return nil
}
