package chat

import (
	"encoding/json"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// ADR 0065: Advanced (no reasoning) is the Advanced contract for a strong
// model run without reasoning. Interactive chat asks for one private basis
// before the edits, in the same generation. The note is inert planning text:
// never a parsed language rule, a constraint, speech, or a motion command.
// Advanced prompts and schemas are unchanged.

const chatPlanIntro = "Before edits, write basis: one or two brief private sentences. Name every change the latest message asks for; an earlier request that current_score does not show was not applied and is not part of this turn unless the person asks to retry it. A requested feel, style or rhythm for the movement is a motion request: encode it now with the closest motion controls instead of declining or asking for confirmation. Name the composition that encodes the request, then check the chosen values against the rules above"

const chatPlanLayeredChecks = ": counts; full-range or full-device travel reaches semantic 0 and 100 unless the human restricted travel, while filling the current band keeps that band's endpoints; do not invent inset margins or mistake the current working band for a human restriction; strokes groups descend at least 10 from each top to the next bottom, including the last group back to the first, so groups in separate regions use regions; alternate_ends keeps widest_percent within half the outer band; pace that differs by location follows the center layer (same: faster at the tip, opposite: faster at the base), and directional_pace only contrasts up and down travel."

const chatPlanClose = " When correcting a rejected response, keep the requested change in a valid form. The basis is private planning; reply remains the normal chat response and never mentions it."

func usesChatPlanNote(capabilities Capabilities, trusted bool) bool {
	return capabilities.ChatPlanNote && capabilities.AdvancedMotion && !trusted && capabilities.Motion &&
		(capabilities.MotionMode == MotionModeLayered || capabilities.MotionMode == MotionModeCreativeV2)
}

func chatPlanGuide(mode MotionMode) string {
	checks := "."
	if mode == MotionModeLayered {
		checks = chatPlanLayeredChecks
	}
	return chatPlanIntro + checks + chatPlanClose
}

// The guide closes the system prompt, after the contract it refers to.
func withChatPlanMessages(messages []llm.Message, mode MotionMode) []llm.Message {
	if len(messages) == 0 || messages[0].Role != "system" {
		return messages
	}
	result := append([]llm.Message(nil), messages...)
	result[0].Content += "\n\n" + chatPlanGuide(mode)
	return result
}

// No length limit is advertised: strict hosted schemas differ in keyword
// support, and an over-long private note is no reason to reject valid edits.
func withChatPlanSchema(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 {
		return schema
	}
	return withPlanNoteSchema(schema, map[string]any{"type": "string"})
}

func takeChatPlanNote(raw string) (string, string, error) {
	root, err := uniqueLayeredObject(json.RawMessage(raw))
	if err != nil {
		return raw, "", err
	}
	value, exists := root["basis"]
	if !exists {
		return raw, "", nil
	}
	var note string
	if json.Unmarshal(value, &note) != nil {
		return raw, "", errors.New("basis must be a private planning note string")
	}
	delete(root, "basis")
	encoded, err := json.Marshal(root)
	return string(encoded), note, err
}
