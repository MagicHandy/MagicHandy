package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// parseHostedLayeredReply translates explicit model choices into the existing
// domain edit. It never resolves competing instructions or invents a correction.
// Recall, lifecycle authority, atomic validation and the shared engine stay on
// the same path used by local Layered replies.
func parseHostedLayeredReply(raw string, current motion.FlowSpec, limits config.MotionSettings) (AssistantResponse, motion.FlowSpec, []string, error) {
	normalized, err := normalizeLayeredOperations(raw)
	if err != nil {
		return AssistantResponse{}, current, nil, err
	}
	response, next, changed, err := ParseLayeredReply(normalized, current, limits)
	return response, next, changed, hostedStrokeLengthError(err)
}

func normalizeLayeredOperations(raw string) (string, error) {
	root, err := uniqueLayeredObject(json.RawMessage(raw))
	if err != nil {
		return "", err
	}
	if _, present := root["edits"]; !present {
		return raw, nil // The domain parser reports a missing edits object.
	}
	edits, err := uniqueLayeredObject(root["edits"])
	if err != nil {
		return "", err
	}
	if _, legacy := edits["change_by"]; legacy {
		return "", errors.New("hosted Layered edits require one set or adjust operation per control; change_by is not allowed")
	}
	if err := normalizeHostedLayerOperations(edits); err != nil {
		return "", err
	}
	if err := normalizeHostedStrokeLengths(edits); err != nil {
		return "", err
	}
	if err := normalizeHostedLayeredComposition(edits); err != nil {
		return "", err
	}
	delete(edits, "composition")
	if rawControls, present := edits["controls"]; present {
		controls, err := uniqueLayeredObject(rawControls)
		if err != nil {
			return "", err
		}
		absolute, relative := map[string]json.RawMessage{}, map[string]json.RawMessage{}
		for name, rawOperation := range controls {
			op, value, err := layeredControlOperation(rawOperation)
			if err != nil {
				return "", fmt.Errorf("control %s: %w", name, err)
			}
			if op == "set" {
				absolute[name] = value
			} else {
				relative[name] = value
			}
		}
		// Preserve explicit zero values and empty control objects. No numeric
		// value, including zero, doubles as an omission or no-change sentinel.
		edits["controls"], _ = json.Marshal(absolute)
		if len(relative) > 0 {
			edits["change_by"], _ = json.Marshal(relative)
		}
	}
	if rawLayers, present := edits["layers"]; present {
		layers, err := normalizeLayeredPeriods(rawLayers)
		if err != nil {
			return "", err
		}
		edits["layers"] = layers
	}
	root["edits"], _ = json.Marshal(edits)
	encoded, err := json.Marshal(root)
	return string(encoded), err
}

func normalizeLayeredPeriods(raw json.RawMessage) (json.RawMessage, error) {
	var layers []json.RawMessage
	if err := json.Unmarshal(raw, &layers); err != nil || layers == nil {
		return nil, errors.New("layers must be an array")
	}
	for i, layer := range layers {
		fields, err := uniqueLayeredObject(layer)
		if err != nil {
			return nil, err
		}
		if _, legacy := fields["period_change_cycles"]; legacy {
			return nil, errors.New("hosted layer timing requires one set or adjust operation in period_cycles")
		}
		if rawPeriod, present := fields["period_cycles"]; present {
			op, value, err := layeredControlOperation(rawPeriod)
			if err != nil {
				return nil, fmt.Errorf("layer period: %w", err)
			}
			if op == "adjust" {
				delete(fields, "period_cycles")
				fields["period_change_cycles"] = value
			} else {
				fields["period_cycles"] = value
			}
		}
		layers[i], _ = json.Marshal(fields)
	}
	return json.Marshal(layers)
}

func layeredControlOperation(raw json.RawMessage) (string, json.RawMessage, error) {
	fields, err := uniqueLayeredObject(raw)
	if err != nil {
		return "", nil, err
	}
	var op string
	if len(fields) != 2 || json.Unmarshal(fields["op"], &op) != nil || (op != "set" && op != "adjust") || len(fields["value"]) == 0 || string(fields["value"]) == "null" {
		return "", nil, errors.New("expected exactly op (set or adjust) and a non-null value")
	}
	return op, fields["value"], nil
}

// Providers without constrained decoding can still return duplicate fields.
// Reject them instead of letting encoding/json silently choose the last edit.
func uniqueLayeredObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, errors.New("expected a Layered JSON object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid Layered field")
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, fmt.Errorf("duplicate Layered field: %s", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid Layered JSON object")
	}
	return fields, nil
}
