package chat

import (
	"encoding/json"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// PromptHistoryLimit retains up to 32 exchanges from the canonical session.
// A separate byte bound keeps verbose sessions from consuming the context.
const PromptHistoryLimit = 64
const maxPromptHistoryBytes = 24000

// History carries speech only; the current authoritative score carries motion.
// Earlier turns used to be replayed as {"edits":[]} envelopes, which put a
// standing example of changing nothing in front of every planning turn and
// misreported turns that did edit. The JSON schema enforces the reply format,
// so the prior turns no longer need to demonstrate it. Replaying legacy
// motion.action envelopes would also teach a conflicting control vocabulary.
func continuousMessages(system string, history []llm.Message, message string) []llm.Message {
	messages := buildMessages(system, spokenHistory(history), message)
	// buildMessages re-serializes assistant speech in the catalog contract;
	// unwrap it again so only the words remain.
	for i := 1; i < len(messages)-1; i++ {
		if messages[i].Role == "assistant" {
			messages[i].Content = spokenReply(messages[i].Content)
		}
	}
	return messages
}

// spokenHistory extracts the reply from any structured assistant turn, since
// evaluators may supply complete prior JSON instead of the stored speech.
func spokenHistory(history []llm.Message) []llm.Message {
	spoken := append([]llm.Message(nil), history...)
	for i, prior := range spoken {
		if prior.Role == "assistant" {
			spoken[i].Content = spokenReply(prior.Content)
		}
	}
	return spoken
}

func spokenReply(content string) string {
	var response struct {
		Reply string `json:"reply"`
	}
	if json.Unmarshal([]byte(content), &response) == nil && response.Reply != "" {
		return response.Reply
	}
	return content
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
