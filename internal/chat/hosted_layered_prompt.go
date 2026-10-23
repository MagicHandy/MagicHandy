package chat

import (
	"encoding/json"
	"strings"
)

// Translate only code-owned examples added by the live lifecycle contract.
// Persona text, history and requests never pass through this function.
func hostedLayeredContract(contract string) string {
	if contract == layeredContract {
		return hostedLayeredBaseContract
	}
	lines := strings.Split(contract, "\n")
	for i, line := range lines {
		lines[i] = hostedLayeredExample(line)
	}
	return strings.Join(lines, "\n")
}

func hostedLayeredExample(line string) string {
	start := strings.Index(line, `{"`)
	if start < 0 {
		return line
	}
	var root map[string]json.RawMessage
	if json.Unmarshal([]byte(line[start:]), &root) != nil || root["edits"] == nil {
		return line
	}
	var edits map[string]json.RawMessage
	if json.Unmarshal(root["edits"], &edits) != nil {
		return line
	}
	controls := map[string]json.RawMessage{}
	for _, group := range []string{"controls", "change_by"} {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(edits[group], &fields)
		for name, value := range fields {
			op := "set"
			if group == "change_by" {
				op = "adjust"
			}
			if _, _, err := layeredControlOperation(value); err != nil {
				value, _ = json.Marshal(map[string]any{"op": op, "value": value})
			}
			controls[name] = value
		}
	}
	if len(controls) > 0 {
		edits["controls"], _ = json.Marshal(controls)
		delete(edits, "change_by")
	}
	convertHostedLayerExample(edits)
	if len(edits) == 0 {
		return line
	}
	composition := "manual"
	if edits["geometry"] != nil {
		composition = "geometry"
	}
	if string(edits["geometry"]) == `"full_slider"` || string(edits["composition"]) == `"full_slider"` {
		composition = "full_slider"
		delete(edits, "geometry")
	}
	edits["composition"], _ = json.Marshal(composition)
	root["edits"], _ = json.Marshal(edits)
	encoded, _ := json.Marshal(root)
	return line[:start] + string(encoded)
}

func convertHostedLayerExample(edits map[string]json.RawMessage) {
	var layers []map[string]json.RawMessage
	var removed []string
	if json.Unmarshal(edits["layers"], &layers) != nil {
		layers = nil
	}
	_ = json.Unmarshal(edits["remove_layers"], &removed)
	if layers == nil && removed == nil {
		return
	}
	axes := map[string]map[string]json.RawMessage{}
	for _, layer := range layers {
		var axis string
		if json.Unmarshal(layer["axis"], &axis) != nil {
			return
		}
		delete(layer, "axis")
		convertHostedPeriodExample(layer)
		value, _ := json.Marshal(layer)
		axes[axis] = map[string]json.RawMessage{"op": json.RawMessage(`"edit"`), "value": value}
	}
	for _, axis := range removed {
		axes[axis] = map[string]json.RawMessage{"op": json.RawMessage(`"remove"`)}
	}
	edits["layers"], _ = json.Marshal(axes)
	delete(edits, "remove_layers")
}

func convertHostedPeriodExample(layer map[string]json.RawMessage) {
	for _, field := range []string{"period_cycles", "period_change_cycles"} {
		if value, present := layer[field]; present {
			op := "set"
			if field == "period_change_cycles" {
				op = "adjust"
			}
			if _, _, err := layeredControlOperation(value); err != nil {
				value, _ = json.Marshal(map[string]any{"op": op, "value": value})
			}
			layer["period_cycles"] = value
		}
	}
	delete(layer, "period_change_cycles")
}
