package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const labEditsGuide = `
This interface accepts reply, controls, change_by, layers and remove_layers. No steps or full score replacement.
"controls" sets only explicit absolute values, as described above. "change_by" changes numeric controls by signed integer percentage points (or cycles for memory_cycles), relative to CURRENT SCORE. For example, 5 points slower is {"change_by":{"speed_percent":-5}}. Do not calculate and emit an absolute speed for that same field. A field may appear in controls OR change_by, never both. Deltas are -100..100 and the result must stay inside saved limits. variation_mode is a name, so it only belongs in controls. A zero range ceiling means the current outer width before a relative adjustment.
On an existing sequence, an absolute speed sets every section to that speed; a relative speed adjusts EACH section by the delta and preserves the differences between sections. Relative edits to global min_percent/max_percent are unsupported while sections exist.
"layers" is an array of up to 3 complete layer edits, each with axis (range, center or pace), amount_percent (0..100), period_cycles (2..32) and phase_percent (0..100). Each supplied axis updates that one existing layer or adds it. UNSUPPLIED LAYERS STAY UNCHANGED. [] changes nothing. "remove_layers" lists only the axes to remove. Never update and remove the same axis. To remove every layer, list range, center and pace. Do not copy unchanged layers or controls into the response.
Write the executable edit first, then a brief reply describing that edit. The reply is only text: saying that you changed something without its corresponding edit does nothing.
Examples of separate requests (never combine these unless the user asks for both):
- A little slower, by 3 points: {"change_by":{"speed_percent":-3},"reply":"Speed reduced by 3 points."}
- Disable pace variation: {"controls":{"pace_variation_percent":0},"reply":"Pace variation is off."}
- Set turn softness to 40: {"controls":{"turn_softness_percent":40},"reply":"Turn softness is 40."}
- Hold a steady beat at 80: {"controls":{"cadence_hold_percent":80},"reply":"Cadence hold is 80."}
- Change only an existing layer's amount: read that axis in CURRENT SCORE, copy its period and phase, and emit only that complete layer with its new amount in layers.
- For example, if CURRENT SCORE has a center layer at amount 15, period 10, phase 40, changing its amount to 25 needs {"layers":[{"axis":"center","amount_percent":25,"period_cycles":10,"phase_percent":40}],"reply":"Only the center layer amount is now 25."}
- Remove the pace layer: {"remove_layers":["pace"],"reply":"Pace layer removed."}
Base pace variation (pace_variation_percent) and the optional pace layer are different controls. Turning off base pace variation sets that control to zero; it does not remove a layer. In change_by, emit only the signed change, such as +4 or -3, never the resulting absolute value.
For a question or a request to keep everything unchanged, return only reply. Otherwise emit the requested controls, change_by, layers or remove_layers before reply. Never invent zero-amount layers to represent preservation. Double-check that each claimed change has an executable field and that no unrelated field is present.
For a compound request, emit every requested control together. Serialize controls keys in this order when present: anchor_percent, cadence_hold_percent, max_percent, memory_cycles, min_percent, pace_variation_percent, range_ceiling_percent, range_floor_percent, speed_percent, turn_softness_percent, variation_mode. In particular emit numeric controls BEFORE variation_mode. Example of a combined change: {"reply":"Tip anchor with softer turns and irregular drift.","controls":{"anchor_percent":100,"turn_softness_percent":40,"variation_mode":"drift"}}. No unrelated layers or removals.`

func parseLabEdits(raw string, current motion.FlowSpec, limits config.MotionSettings) (string, motion.FlowSpec, []string, error) {
	var proposal struct {
		Reply        string                     `json:"reply"`
		Controls     map[string]json.RawMessage `json:"controls"`
		Adjust       map[string]json.RawMessage `json:"change_by"`
		Layers       json.RawMessage            `json:"layers"`
		RemoveLayers json.RawMessage            `json:"remove_layers"`
	}
	if err := decodeLabObject(raw, &proposal); err != nil {
		return "", current, nil, err
	}
	if strings.TrimSpace(proposal.Reply) == "" || len(proposal.Reply) > 2000 {
		return "", current, nil, errors.New("a brief reply is required")
	}
	encoded, _ := json.Marshal(current)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	if err := mergeLabControls(fields, proposal.Controls); err != nil {
		return "", current, nil, err
	}
	if err := mergeLabAdjustments(fields, proposal.Controls, proposal.Adjust, current); err != nil {
		return "", current, nil, err
	}
	encoded, _ = json.Marshal(fields)
	var next motion.FlowSpec
	if err := decodeLabObject(string(encoded), &next); err != nil {
		return "", current, nil, err
	}
	if len(current.Steps) > 0 {
		if err := applyExistingSectionControls(&next, proposal.Controls); err != nil {
			return "", current, nil, err
		}
		if delta, okay := proposal.Adjust["speed_percent"]; okay {
			var value int
			_ = json.Unmarshal(delta, &value) // validated by mergeLabAdjustments
			for index := range next.Steps {
				next.Steps[index].SpeedPercent = current.Steps[index].SpeedPercent + value
			}
		}
	}
	if err := mergeLabLayerEdits(&next, proposal.Layers, proposal.RemoveLayers, limits); err != nil {
		return "", current, nil, err
	}
	if err := next.Validate(limits); err != nil {
		return "", current, nil, err
	}
	return proposal.Reply, next, labChangedControls(current, next), nil
}

func mergeLabAdjustments(fields, controls, adjustments map[string]json.RawMessage, current motion.FlowSpec) error {
	for key, raw := range adjustments {
		if _, overlap := controls[key]; overlap {
			return fmt.Errorf("cannot set and adjust the same control: %s", key)
		}
		if key == "variation_mode" {
			return errors.New("variation_mode requires an absolute choice")
		}
		// Reuse the strict allowlist; null and unknown fields stay invalid.
		if err := mergeLabControls(map[string]json.RawMessage{}, map[string]json.RawMessage{key: raw}); err != nil {
			return err
		}
		var delta int
		if err := json.Unmarshal(raw, &delta); err != nil || delta < -100 || delta > 100 {
			return errors.New("relative edits require integer deltas within -100–100")
		}
		if len(current.Steps) > 0 && (key == "min_percent" || key == "max_percent") {
			return errors.New("change section ranges with the sequence interface")
		}
		var before int
		if len(fields[key]) > 0 {
			if err := json.Unmarshal(fields[key], &before); err != nil {
				return err
			}
		}
		if key == "range_ceiling_percent" && before == 0 {
			before = current.MaxPercent - current.MinPercent
		}
		fields[key], _ = json.Marshal(before + delta)
	}
	return nil
}

func mergeLabLayerEdits(next *motion.FlowSpec, rawEdits, rawRemove json.RawMessage, limits config.MotionSettings) error {
	var edits []motion.FlowLayer
	if len(rawEdits) > 0 {
		if err := labRequiredItems(rawEdits, []string{"axis", "amount_percent", "period_cycles", "phase_percent"}); err != nil {
			return err
		}
		if err := decodeLabObject(string(rawEdits), &edits); err != nil {
			return err
		}
	}
	// Validate proposed axes before upserting so duplicates cannot be hidden.
	check := *next
	check.Layers = edits
	if err := check.Validate(limits); err != nil {
		return err
	}
	remove, err := labRemovedAxes(rawRemove, edits)
	if err != nil {
		return err
	}
	next.Layers = slices.DeleteFunc(next.Layers, func(layer motion.FlowLayer) bool { return slices.Contains(remove, layer.Axis) })
	for _, edit := range edits {
		index := slices.IndexFunc(next.Layers, func(layer motion.FlowLayer) bool { return layer.Axis == edit.Axis })
		if index < 0 {
			next.Layers = append(next.Layers, edit)
		} else {
			next.Layers[index] = edit
		}
	}
	return nil
}

func labRemovedAxes(raw json.RawMessage, edits []motion.FlowLayer) ([]string, error) {
	remove := []string{}
	if len(raw) > 0 {
		if string(raw) == "null" {
			return nil, errors.New("remove_layers must be an array of layer axes")
		}
		if err := json.Unmarshal(raw, &remove); err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	for _, axis := range remove {
		if seen[axis] || (axis != "range" && axis != "center" && axis != "pace") {
			return nil, errors.New("remove_layers requires distinct range, center or pace axes")
		}
		if slices.ContainsFunc(edits, func(layer motion.FlowLayer) bool { return layer.Axis == axis }) {
			return nil, errors.New("cannot update and remove the same layer")
		}
		seen[axis] = true
	}
	return remove, nil
}
