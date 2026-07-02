package chat

import (
	"fmt"
	"strings"
)

// PromptSet contains the behavior instructions for one chat profile. The
// machine JSON contract is never part of a set: ComposeSystem appends it in
// code so prompt edits cannot weaken or change it.
type PromptSet struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	System  string `json:"system"`
	Builtin bool   `json:"builtin"`
}

// DefaultPromptSetID is the bundled behavior profile used when the selected
// set is missing.
const DefaultPromptSetID = "magichandy_motion_v1"

// ContractInstructions is the response contract appended to every system
// prompt by code. User-editable prompt sets can change persona and tone, but
// never this contract (IMPLEMENTATION_PLAN.md Phase 10 rule).
const ContractInstructions = `Return exactly one JSON object and no markdown, code fences, prose outside JSON, or extra keys.

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
- Keep speeds conservative unless the user explicitly asks otherwise.`

var builtinPromptSets = []PromptSet{
	{
		ID:      DefaultPromptSetID,
		Name:    "MagicHandy Motion (default)",
		Builtin: true,
		System: strings.TrimSpace(`You are MagicHandy's local motion assistant. Be warm, concise, and
attentive to what the user asks for. Match the user's energy without
escalating beyond their requests.`),
	},
}

// BuiltinPromptSets returns the read-only bundled prompt sets.
func BuiltinPromptSets() []PromptSet {
	sets := make([]PromptSet, len(builtinPromptSets))
	copy(sets, builtinPromptSets)
	return sets
}

// BuiltinPromptSetByID returns a bundled prompt set by identifier.
func BuiltinPromptSetByID(id string) (PromptSet, bool) {
	trimmed := strings.TrimSpace(id)
	for _, set := range builtinPromptSets {
		if set.ID == trimmed {
			return set, true
		}
	}
	return PromptSet{}, false
}

// ComposeSystem builds the full system prompt: behavior text from the set,
// then the code-owned contract, then enabled memories when present.
func ComposeSystem(set PromptSet, memories []string) string {
	var builder strings.Builder
	behavior := strings.TrimSpace(set.System)
	if behavior == "" {
		fallback, _ := BuiltinPromptSetByID(DefaultPromptSetID)
		behavior = fallback.System
	}
	builder.WriteString(behavior)
	builder.WriteString("\n\n")
	builder.WriteString(ContractInstructions)

	if len(memories) > 0 {
		builder.WriteString("\n\nSaved user memories (reference naturally when relevant; never recite the list):")
		for _, memoryText := range memories {
			trimmed := strings.TrimSpace(memoryText)
			if trimmed == "" {
				continue
			}
			builder.WriteString("\n- ")
			builder.WriteString(trimmed)
		}
	}
	return builder.String()
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
