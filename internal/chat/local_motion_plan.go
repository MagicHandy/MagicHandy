package chat

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// ADR 0060 records the deliberate Autopilot latency/quality trade-off. Keep
// interactive chat and Advanced contracts off this path. The note is part of
// one generation, not an extra request; its text never controls the device.

const localPlanGuide = "Before edits, write basis: one short planning sentence naming the human constraints that still apply, any restriction released or expired, and what may change now. Then emit only edits consistent with that basis. The basis is internal planning; reply remains the normal chat response. No fixed speed, location or variation is preferred."
const localHumanDirectionGuide = "Judge which wishes are still active before proposing edits. An ongoing exact hold means empty edits, including no evolution. A fixed speed leaves other dimensions free. Release restrictions when the human releases them or their stated duration ends."
const localPlanNoteLimit = 240
const localPlanOutputFloor = 384

func localDecisionNotes(caps Capabilities) bool {
	return !caps.AdvancedMotion && (caps.SimpleMotion || caps.IntermediateMotion)
}

func localPlanningMessages(messages []llm.Message, state MotionContext) []llm.Message {
	if len(messages) == 0 {
		return messages
	}
	result := append([]llm.Message(nil), messages...)
	result[0].Content += "\n\n" + localPlanGuide
	if len(state.UserRequests) > 0 {
		recent, _ := json.Marshal(timedUserRequests(state))
		label := "\n\nHuman direction (oldest first, age in seconds): "
		if len(state.UserRequestSecondsAgo) != len(state.UserRequests) {
			label = "\n\nHuman direction (oldest first; ages unknown): "
		}
		result[len(result)-1].Content += label + string(recent) + "\n" + localHumanDirectionGuide
	}
	return result
}

// A note precedes edits in the constrained decoder. It is inert model metadata,
// never a parsed language rule, constraint lock, or independent motion command.
func withLocalPlanSchema(schema json.RawMessage) json.RawMessage {
	var root map[string]any
	if json.Unmarshal(schema, &root) != nil {
		return schema
	}
	var add func(map[string]any)
	add = func(object map[string]any) {
		for _, key := range []string{"oneOf", "anyOf"} {
			if branches, ok := object[key].([]any); ok {
				for _, branch := range branches {
					if b, ok := branch.(map[string]any); ok {
						add(b)
					}
				}
				return
			}
		}
		if fields, ok := object["properties"].(map[string]any); ok && fields["edits"] != nil {
			fields["basis"] = map[string]any{"type": "string", "maxLength": localPlanNoteLimit}
			required, _ := object["required"].([]any)
			object["required"] = append(required, "basis")
		}
	}
	add(root)
	encoded, _ := json.Marshal(root)
	return encoded
}

func takeLocalPlanNote(raw string) (string, error) {
	root, err := uniqueLayeredObject(json.RawMessage(raw))
	if err != nil {
		return raw, err
	}
	value, exists := root["basis"]
	if !exists {
		return raw, nil
	} // Older/unconstrained clients retain the normal contract.
	var note string
	if json.Unmarshal(value, &note) != nil || string(value) == "null" || utf8.RuneCountInString(note) > localPlanNoteLimit {
		return raw, errors.New("basis must be a short decision note, at most 240 characters")
	}
	delete(root, "basis")
	encoded, err := json.Marshal(root)
	return string(encoded), err
}

func localPlanningTokens(tokens int) int {
	if tokens > 0 {
		return max(tokens, localPlanOutputFloor)
	}
	return tokens // Preserve the ordinary service default for unspecified budgets.
}
