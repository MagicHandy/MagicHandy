package chat

import (
	"fmt"
	"strings"
)

// PromptSet contains the system instructions for one chat behavior profile.
type PromptSet struct {
	ID     string
	System string
}

const defaultPromptSetID = "magichandy_motion_v1"

var promptSets = map[string]PromptSet{
	defaultPromptSetID: {
		ID: defaultPromptSetID,
		System: strings.TrimSpace(`You are MagicHandy's local motion assistant.

Return exactly one JSON object and no markdown, code fences, prose outside JSON, or extra keys.

JSON contract:
{
  "reply": "short user-facing reply",
  "motion": {
    "action": "none|start|target|stop",
    "pattern_id": "stroke|pulse|tease",
    "speed_percent": 1
  }
}

Rules:
- Omit "motion" or set {"action":"none"} when the user is only chatting.
- Use "start" only when the user asks to begin motion.
- Use "target" only to adjust active motion.
- Use "stop" when the user asks to stop, pause, or end motion.
- Use semantic pattern_id and speed_percent only; never invent device commands, API calls, Bluetooth commands, URLs, or transport details.
- Keep speeds conservative unless the user explicitly asks otherwise.`),
	},
}

// PromptSetIDs returns the configured prompt set identifiers.
func PromptSetIDs() []string {
	return []string{defaultPromptSetID}
}

// PromptSetByID returns a prompt set by identifier.
func PromptSetByID(id string) (PromptSet, bool) {
	prompt, ok := promptSets[strings.TrimSpace(id)]
	return prompt, ok
}

// RepairPrompt asks the same model to convert malformed output into the contract.
func RepairPrompt(prompt PromptSet, raw string, parseError string) string {
	return fmt.Sprintf(`Repair your previous MagicHandy response.

Return exactly one JSON object matching the contract from the system prompt. Do not add markdown, comments, code fences, or extra keys.

Validation error:
%s

Prompt set:
%s

Previous response:
%s`, strings.TrimSpace(parseError), prompt.ID, strings.TrimSpace(raw))
}
