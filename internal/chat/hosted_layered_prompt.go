package chat

import (
	"encoding/json"
	"strings"
)

const hostedLayeredControlsGuide = `- controls: each requested control has exactly one operation: {"op":"set","value":<absolute value>} OR {"op":"adjust","value":<signed delta>}. Never emit change_by. Preserve unrequested controls by omitting them (or null only when the output schema requires nullable fields). Do not fill them with copied current values or zero deltas. Zero is a real value, not a placeholder.
Absolute bounds: min_percent 0..90, max_percent 10..100, speed_percent saved limits, anchor_percent 0..100, memory_cycles 2..32, pace_variation_percent 0..40, variation_mode drift or waves. Numeric adjustments are -100..100; variation_mode only supports set. A delta changes the current value; do not also emit its calculated absolute result. Resolve compound requests by dimension: permission to develop location or reach does not authorize changing a pace the person has asked to retain.
current_score.controls contains plain state values, not edit operations. Examples below describe only requested changes.`

const hostedLayeredPeriodGuide = `For layer timing use period_cycles:{"op":"set","value":<2..32>} or period_cycles:{"op":"adjust","value":<-30..30>}. Never emit period_change_cycles. Increasing an existing pace period BY 4 cycles uses {"op":"adjust","value":4}: current 22 becomes 26, not 4. Slower development changes period, not amount or speed. Omit unchanged timing (or null only when required by the output schema).`

const hostedLayeredLayersGuide = `- layers: an object keyed by range, center and pace. Each chosen axis has one operation: {"op":"edit","value":{...partial fields...}} patches amount_percent (0..100), period_cycles (set/adjust), phase_percent (0..100), or shape (drift/alternate/wave); {"op":"remove"} removes it. Choose the operation before its value. Omitted axes stay unchanged. Never emit remove_layers or an axis field inside the operation. A removed axis has no value. New defaults: amount 30, period 12, phase 0, drift.`

const hostedLayeredGeometryGuide = `Composition: choose a named geometry OR manual refinements.
For a named operation, edits.composition is "geometry" and edits.geometry names it. That operation supplies its coupled shape controls, widths and range/center layers. Do not restate those fields. You may additionally change speed_percent, memory_cycles, pace_variation_percent, or the pace layer. Outer min_percent/max_percent are also available except with full_slider, which sets them itself. Geometry composition has no stroke_width, anchor_percent, variation_mode, or range/center layer fields.
Full-slider travel is a distinct composition: {"composition":"full_slider"}, with no geometry field. Choose it only for intended travel over 0..100; it replaces the outer band. The other named geometries, including full_band, use composition "geometry" and operate inside the chosen band.
For custom shape or layer refinements, edits.composition is "manual" and geometry is omitted. Every control, custom stroke_width and layer edit is available in this branch. Use it for a custom anchor, custom widths, changes to a range/center layer, or combinations that differ from a named geometry's definition. It also handles independent pace edits. The base width envelope follows variation_mode (waves is periodic, drift is irregular); a drift range layer does not replace that underlying mode. This is a partial patch, not a snapshot: omit unrelated values and layers instead of copying the current score.
Plan from current_score, not from the schema's numeric bounds: saved_limits are permission limits, never targets. Check each proposed operation against the request, including the resulting pace and outer band. A geometry-only request normally needs no controls, pace-layer edits or evolution. The reply must describe the actual resulting score; claiming a value stayed unchanged cannot undo an emitted change.
For no change, edits is {}. Pace stays independent of shape: change it only when the current request or autonomous permission calls for it. Use null only when the strict schema requires an unchosen field. A custom alternating-end example (current full band):
{"edits":{"stroke_width":{"min_percent":20,"max_percent":40},"layers":[{"axis":"center","amount_percent":100,"period_cycles":8,"shape":"alternate"}],"remove_layers":["range"]},"reply":"Custom short strokes alternate between the ends."}
`

// hostedLayeredContract translates only the code-owned contract and its JSON
// examples. Persona text, history and the user's request never pass through it.
// It is idempotent because interactive composition adds state-specific examples
// after the common contract is composed. Local prompt bytes remain untouched.
func hostedLayeredContract(contract string) string {
	lines := strings.Split(contract, "\n")
	kept := make([]string, 0, len(lines))
	hasGeometryGuide := strings.Contains(contract, "Composition: choose a named geometry OR manual refinements.")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "Use geometry for coupled reach/location edits."):
			line = "Choose the composition method for coupled reach/location edits. Named geometry preserves pace; additionally change speed_percent only when requested or autonomously authorized. Decide the intended shape and preserved dimensions before emitting independent controls."
		case !hasGeometryGuide && strings.HasPrefix(line, "Geometry works inside the outer band."):
			line += "\n" + hostedLayeredGeometryGuide
		case strings.HasPrefix(line, "- controls: partial absolute"):
			line = hostedLayeredControlsGuide
		case strings.HasPrefix(line, "- change_by:"):
			continue
		case strings.HasPrefix(line, "For relative timing use period_change_cycles"):
			line = hostedLayeredPeriodGuide
		case strings.HasPrefix(line, "- layers: partial axis edits"):
			line = hostedLayeredLayersGuide
		}
		for _, part := range strings.Split(line, "\n") {
			kept = append(kept, hostedLayeredExample(part))
		}
	}
	return strings.Join(kept, "\n")
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
