package chat

import (
	"encoding/json"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func validateDirectionalPaceObject(raw json.RawMessage) error {
	fields, err := uniqueLayeredObject(raw)
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return errors.New("directional_pace requires exactly up_rate_percent and down_rate_percent")
	}
	for _, key := range []string{"up_rate_percent", "down_rate_percent"} {
		var value int
		if json.Unmarshal(fields[key], &value) != nil || value < 1 || value > 100 {
			return errors.New("directional_pace requires explicit integer up_rate_percent and down_rate_percent within 1..100")
		}
	}
	return nil
}

func hostedDirectionalPaceSchema() map[string]any {
	rate := map[string]any{"type": "integer", "minimum": 1, "maximum": 100}
	return map[string]any{"type": "object", "additionalProperties": false,
		"properties": map[string]any{"up_rate_percent": rate, "down_rate_percent": rate},
		"required":   []string{"up_rate_percent", "down_rate_percent"}}
}

const hostedDirectionalPaceGuide = `Direction-specific pace is available in every Layered composition through directional_pace:{up_rate_percent,down_rate_percent}. Up travels toward the tip (100); down toward the base (0), regardless of current region or anchor. Each rate is 1..100% of the selected base travel rate, multiplied with group rates and existing pace modulation. Use it for a quick downstroke with a slower return, or its reverse; a pace wave or alternating fast/slow groups is not tied to travel direction. Choose contrast for the request without a preferred ratio; base speed serves the faster leg. Omit the block to preserve it, or set both rates to 100 to remove directional contrast. Direction changes keep the shared rounded turns and device limits; rapid travel never authorizes a dwell, impact or a bypass of the fitter. Preserve reach/counts and unrelated timing unless requested.`

func validateDirectionCapability(raw string, caps Capabilities) error {
	if caps.AdvancedMotion || caps.DirectionalPace {
		return nil
	}
	var root struct {
		Edits map[string]json.RawMessage `json:"edits"`
	}
	if json.Unmarshal([]byte(raw), &root) == nil && root.Edits["directional_pace"] != nil {
		return errors.New("direction-specific Layered pace requires Advanced model capability")
	}
	return nil
}

func applyLayeredDirectionalPace(next *motion.FlowSpec, value *motion.DirectionalPace) {
	if value == nil {
		return
	}
	pace := *value
	next.DirectionalPace = &pace
	if pace.UpRatePercent == 100 && pace.DownRatePercent == 100 {
		next.DirectionalPace = nil
	}
}
