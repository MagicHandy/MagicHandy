package chat

import (
	"encoding/json"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// PromptHistoryLimit retains up to 32 exchanges from the canonical session.
// A separate byte bound keeps verbose sessions from consuming the context.
const PromptHistoryLimit = 64
const maxPromptHistoryBytes = 24000

// Continuous chat must see examples of its own response format. Replaying
// legacy motion.action envelopes teaches a conflicting control vocabulary.
// History carries speech only; the current authoritative score carries motion.
func continuousMessages(system string, history []llm.Message, message string, mode MotionMode) []llm.Message {
	// Internal evaluators may supply complete prior JSON instead of the plain
	// speech stored by production. Extract that speech before legacy sanitation
	// can quote the whole old edit object as a reply.
	spokenHistory := append([]llm.Message(nil), history...)
	for i, prior := range spokenHistory {
		var response struct {
			Reply string `json:"reply"`
		}
		if prior.Role == "assistant" && json.Unmarshal([]byte(prior.Content), &response) == nil && response.Reply != "" {
			spokenHistory[i].Content = response.Reply
		}
	}
	messages := buildMessages(system, spokenHistory, message)
	for i := 1; i < len(messages)-1; i++ {
		if messages[i].Role != "assistant" {
			continue
		}
		var previous struct {
			Reply string `json:"reply"`
		}
		if json.Unmarshal([]byte(messages[i].Content), &previous) != nil {
			continue
		}
		var edits any = map[string]any{}
		if mode == MotionModeCreativeV2 {
			edits = []any{}
		}
		encoded, _ := json.Marshal(map[string]any{"edits": edits, "reply": previous.Reply})
		messages[i].Content = string(encoded)
	}
	return messages
}

func continuousOutputGuard(capabilities Capabilities) string {
	empty := "{}"
	if capabilities.MotionMode == MotionModeCreativeV2 {
		empty = "[]"
	}
	guard := `FINAL OUTPUT RULE: Return one JSON object with "edits" and "reply", using this mode's contract. Ordinary conversation and questions require "edits":` + empty + `. Do not evolve during ordinary conversation. Use the selected voice for reply; technical examples illustrate edits, not a required speaking style. Never output a "motion" field. Close JSON immediately after the final sentence, with no trailing blank lines.`
	if capabilities.MoodTracking {
		guard += ` Optional "new_mood" may use a listed mood.`
	}
	return guard
}

func promptOutputGuard(capabilities Capabilities) string {
	if capabilities.Motion && (capabilities.MotionMode == MotionModeLayered || capabilities.MotionMode == MotionModeCreativeV2) {
		return continuousOutputGuard(capabilities)
	}
	if capabilities.MoodTracking {
		return finalOutputGuardWithMood
	}
	return finalOutputGuard
}
