package chat

import (
	"encoding/json"
	"errors"
	"strings"
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

// Domain validators deliberately keep their native field names. Translate only
// their unambiguous width diagnostics at the Advanced schema boundary, never
// position min/max fields or the person's language. Preserve the cause so typed
// geometry conflicts can still be inspected with errors.As.
var hostedWidthDiagnosticNames = strings.NewReplacer(
	"stroke_width requires both min_percent and max_percent, with 10 <= min <= max", "stroke_width requires both shortest_percent and widest_percent, with 10 <= shortest_percent <= widest_percent",
	"stroke_width.min_percent", "stroke_width.shortest_percent",
	"stroke_width.max_percent", "stroke_width.widest_percent",
	"flow range_floor_percent", "stroke_width.shortest_percent",
	"flow range ceiling must be zero for the outer width or between the floor and outer width", "stroke_width.widest_percent must be between shortest_percent and the outer width (max_percent - min_percent)",
)

type hostedWidthDiagnostic struct {
	cause error
	text  string
}

func (e *hostedWidthDiagnostic) Error() string { return e.text }
func (e *hostedWidthDiagnostic) Unwrap() error { return e.cause }

func hostedStrokeLengthError(err error) error {
	if err == nil {
		return nil
	}
	text := hostedWidthDiagnosticNames.Replace(err.Error())
	if text == err.Error() {
		return err
	}
	return &hostedWidthDiagnostic{cause: err, text: text}
}
