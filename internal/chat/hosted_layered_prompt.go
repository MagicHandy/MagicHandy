package chat

import (
	"encoding/json"
	"strings"
)

const hostedLayeredControlsGuide = `- controls: each requested control has exactly one operation: {"op":"set","value":<absolute value>} OR {"op":"adjust","value":<signed delta>}. Never emit change_by. Preserve unrequested controls by omitting them (or null only when the output schema requires nullable fields). Do not fill them with copied current values or zero deltas. Zero is a real value, not a placeholder.
Absolute bounds: min_percent 0..90, max_percent 10..100, speed_percent saved limits, anchor_percent 0..100, memory_cycles 2..32, pace_variation_percent 0..40, variation_mode drift or waves. Numeric adjustments are -100..100; variation_mode only supports set. A delta changes the current value; do not also emit its calculated absolute result. Gentler primarily reduces speed; jerk/hammer do not authorize a speed increase.
current_score.controls contains plain state values, not edit operations. Examples below describe only requested changes.`

const hostedLayeredPeriodGuide = `For layer timing use period_cycles:{"op":"set","value":<2..32>} or period_cycles:{"op":"adjust","value":<-30..30>}. Never emit period_change_cycles. Increasing an existing pace period BY 4 cycles uses {"op":"adjust","value":4}: current 22 becomes 26, not 4. Slower development changes period, not amount or speed. Omit unchanged timing (or null only when required by the output schema).`

// hostedLayeredContract translates only the code-owned contract and its JSON
// examples. Persona text, history and the user's request never pass through it.
// It is idempotent because interactive composition adds state-specific examples
// after the common contract is composed. Local prompt bytes remain untouched.
func hostedLayeredContract(contract string) string {
	lines := strings.Split(contract, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "- controls: partial absolute"):
			line = hostedLayeredControlsGuide
		case strings.HasPrefix(line, "- change_by:"):
			continue
		case strings.HasPrefix(line, "For relative timing use period_change_cycles"):
			line = hostedLayeredPeriodGuide
		case strings.HasPrefix(line, "- layers: partial axis edits"):
			line = strings.Replace(line, "period_cycles (2..32)", "period_cycles (set or adjust operation, described below)", 1)
		}
		kept = append(kept, hostedLayeredExample(line))
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
	var layers []map[string]json.RawMessage
	if json.Unmarshal(edits["layers"], &layers) == nil && layers != nil {
		for _, layer := range layers {
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
		edits["layers"], _ = json.Marshal(layers)
	}
	if len(edits) == 0 {
		return line
	}
	root["edits"], _ = json.Marshal(edits)
	encoded, _ := json.Marshal(root)
	return line[:start] + string(encoded)
}
