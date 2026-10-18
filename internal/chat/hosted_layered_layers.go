package chat

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
)

// Each axis gets one explicit edit/remove choice. The existing domain validator
// remains responsible for bounds, distinct axes and geometric compatibility.
func normalizeHostedLayerOperations(edits map[string]json.RawMessage) error {
	raw, present := edits["layers"]
	if !present || !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		return nil
	}
	if _, legacy := edits["remove_layers"]; legacy {
		return errors.New("layer operations cannot be mixed with remove_layers")
	}
	axes, err := uniqueLayeredObject(raw)
	if err != nil {
		return err
	}
	layers := []map[string]json.RawMessage{}
	remove := []string{}
	for _, axis := range slices.Sorted(maps.Keys(axes)) {
		raw := axes[axis]
		fields, err := uniqueLayeredObject(raw)
		if err != nil {
			return err
		}
		var op string
		if json.Unmarshal(fields["op"], &op) != nil {
			return errors.New("layer operation must be edit or remove")
		}
		delete(fields, "op")
		switch op {
		case "remove":
			if len(fields) != 0 {
				return errors.New("a removed layer cannot also contain edits")
			}
			remove = append(remove, axis)
		case "edit":
			if len(fields) != 1 || fields["value"] == nil {
				return errors.New("layer edit requires exactly one value object")
			}
			patch, err := uniqueLayeredObject(fields["value"])
			if err != nil {
				return err
			}
			if _, duplicate := patch["axis"]; duplicate {
				return errors.New("layer axis is selected by its key")
			}
			patch["axis"], _ = json.Marshal(axis)
			layers = append(layers, patch)
		default:
			return errors.New("layer operation must be edit or remove")
		}
	}
	edits["layers"], _ = json.Marshal(layers)
	if len(remove) > 0 {
		edits["remove_layers"], _ = json.Marshal(remove)
	}
	return nil
}
