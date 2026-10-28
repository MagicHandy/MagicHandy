package chat

import (
	"encoding/json"
	"errors"
)

// These are grammar checks on explicit structured edits, never language rules.
// Missing composition remains compatible with unambiguous earlier responses;
// a selected geometry still cannot be paired with competing manual shape edits.
func normalizeHostedLayeredComposition(edits map[string]json.RawMessage) error {
	if string(edits["composition"]) == `"phrase"` {
		return validateHostedPhraseComposition(edits)
	}
	if edits["phrase"] != nil {
		return errors.New("a counted phrase requires phrase composition")
	}
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
	// Do not reject controls solely because geometry also selects them. The
	// shared applyLayeredGeometryEdit compares their resolved values against
	// that geometry, including relative edits against the actual recalled base.
	// Agreeing endpoints/anchors are redundant; differing values still fail.
	return validateHostedGeometryLayers(edits)
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
