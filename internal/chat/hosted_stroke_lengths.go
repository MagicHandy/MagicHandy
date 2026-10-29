package chat

import (
	"encoding/json"
	"errors"
)

const hostedStrokeLengthGuide = `Stroke widths are TRAVEL LENGTHS, not positions: 10 <= shortest_percent <= widest_percent <= (outer max_percent - outer min_percent). Outer endpoints are positions. Do not copy position values into stroke lengths.`

// Translate explicit length names only; values and geometric validation are
// unchanged. Legacy persisted responses retain their original domain spelling.
func normalizeHostedStrokeLengths(edits map[string]json.RawMessage) error {
	raw, present := edits["stroke_width"]
	if !present {
		return nil
	}
	width, err := uniqueLayeredObject(raw)
	if err != nil {
		return err
	}
	if width["shortest_percent"] == nil && width["widest_percent"] == nil {
		return nil
	}
	if len(width) != 2 || width["shortest_percent"] == nil || width["widest_percent"] == nil {
		return errors.New("stroke_width requires shortest_percent and widest_percent lengths without competing fields")
	}
	edits["stroke_width"], err = json.Marshal(map[string]json.RawMessage{"min_percent": width["shortest_percent"], "max_percent": width["widest_percent"]})
	return err
}

func hostedStrokeLengthContext(context map[string]any) {
	if width, ok := context["stroke_width"].(LayeredStrokeWidth); ok {
		context["stroke_width"] = map[string]int{"shortest_percent": width.MinPercent, "widest_percent": width.MaxPercent}
	}
}
