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
			return errors.New("layer operation must be edit, remove or pace follow")
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
		case "follow":
			patch, err := normalizeHostedFollowLayer(axis, fields)
			if err != nil {
				return err
			}
			layers = append(layers, patch)
		default:
			return errors.New("layer operation must be edit, remove or pace follow")
		}
	}
	edits["layers"], _ = json.Marshal(layers)
	if len(remove) > 0 {
		edits["remove_layers"], _ = json.Marshal(remove)
	}
	return nil
}

func normalizeHostedFollowLayer(axis string, fields map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if axis != "pace" || len(fields) != 1 || fields["value"] == nil {
		return nil, errors.New("only pace supports a follow operation with one value")
	}
	patch, err := uniqueLayeredObject(fields["value"])
	if err != nil {
		return nil, err
	}
	if len(patch) != 4 || patch["axis"] == nil || patch["relation"] == nil || patch["amount_percent"] == nil || patch["low_rate_percent"] == nil {
		return nil, errors.New("pace follow requires axis, relation, amount_percent and low_rate_percent")
	}
	follow, _ := json.Marshal(map[string]json.RawMessage{"axis": patch["axis"], "relation": patch["relation"]})
	return map[string]json.RawMessage{"axis": json.RawMessage(`"pace"`), "follow": follow,
		"amount_percent": patch["amount_percent"], "low_rate_percent": patch["low_rate_percent"]}, nil
}
