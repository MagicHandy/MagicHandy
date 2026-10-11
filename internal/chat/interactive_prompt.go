package chat

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// ComposeInteractivePrompt includes the lifecycle contract used by live chat.
// Autonomous planning and experimental Lab prompts intentionally use their own
// authority; they must not acquire interactive start permission from this path.
func ComposeInteractivePrompt(set PromptSet, memories []string, patterns []PatternChoice, capabilities Capabilities, state *MotionContext, conversation *ConversationContext, budget ...PromptBudgetSettings) PromptComposition {
	composition := ComposePrompt(set, memories, patterns, capabilities, state, conversation, budget...)
	if !capabilities.Motion || (capabilities.MotionMode != MotionModeCreativeV2 && capabilities.MotionMode != MotionModeLayered) {
		return composition
	}
	current := MotionContext{MotionMode: capabilities.MotionMode}
	if state != nil {
		current = *state
	}
	sections := make([]PromptSection, 0, len(composition.Sections)+1)
	for _, section := range composition.Sections {
		if section.ID == "response_contract" {
			section.Text = continuousInteractiveContract(section.Text, current)
			section.Characters = utf8.RuneCountInString(section.Text)
			section.Bytes = len(section.Text)
		}
		if section.ID == "output_guard" {
			sections = appendPromptSection(sections, "live_action", "Live chat action", continuousActionGuideFor(current))
		}
		sections = append(sections, section)
	}
	texts := make([]string, 0, len(sections))
	for _, section := range sections {
		texts = append(texts, section.Text)
	}
	composition.Sections = sections
	composition.Prompt = strings.Join(texts, "\n\n")
	composition.Characters = utf8.RuneCountInString(composition.Prompt)
	composition.Bytes = len(composition.Prompt)
	return composition
}

// Experimental examples omit action. Replaying those beside a mandatory live
// action field encourages unconstrained models to copy an invalid envelope.
// Use state-valid examples without inferring authority from a returned edit.
func continuousInteractiveContract(contract string, state MotionContext) string {
	contract = continuousInteractiveExamples(contract, state)
	if state.Autopilot {
		contract = strings.ReplaceAll(contract, `,"reply":`, `,"stay_unchanged":false,"reply":`)
	}
	return contract
}

func continuousInteractiveExamples(contract string, state MotionContext) string {
	if state.Running && !state.Paused {
		// Keep the established motion examples for active local sessions. Only
		// fix their envelope; removing useful geometry examples harms smaller models.
		contract = strings.ReplaceAll(contract, `{"edits":[]`, `{"action":"none","edits":[]`)
		contract = strings.ReplaceAll(contract, `{"edits":{}`, `{"action":"none","edits":{}`)
		return strings.ReplaceAll(contract, `{"edits":`, `{"action":"update","edits":`)
	}
	if state.MotionMode == MotionModeLayered {
		if begin := strings.Index(contract, "\nExamples:"); begin >= 0 {
			if end := strings.Index(contract[begin:], "Independent refinements"); end >= 0 {
				contract = contract[:begin] + "\n\n" + contract[begin+end:]
			}
		}
		contract = strings.ReplaceAll(contract, `Output {"edits":{...},"reply":"..."}.`, `Output {"action":"none|start|update","edits":{...},"reply":"..."}; choose one action permitted by the current state.`)
		const relativeExample = `Example: increase the pace period BY 4 cycles -> {"edits":{"layers":[{"axis":"pace","period_change_cycles":4}]},"reply":"Pace variation unfolds four cycles more gradually."} `
		contract = strings.ReplaceAll(contract, relativeExample, "For example, increasing the pace period BY 4 cycles uses period_change_cycles:4. ")
	}
	if begin := strings.Index(contract, "These examples illustrate edit mechanics"); begin >= 0 {
		if end := strings.Index(contract[begin:], "Emit every requested group"); end >= 0 {
			contract = contract[:begin] + contract[begin+end:]
		}
	}
	contract = strings.ReplaceAll(contract, `Output {"edits":[...],"reply":"..."}.`, `Output {"action":"none|start|update","edits":[...],"reply":"..."}; choose one action permitted by the current state.`)
	contract = strings.ReplaceAll(contract, `Questions or ordinary conversation require edits:[].`, `Questions or ordinary conversation require action "none" and edits:[].`)
	empty := "{}"
	if state.MotionMode == MotionModeCreativeV2 {
		empty = "[]"
	}
	contract += "\n\nLive chat envelope examples (reply wording follows the selected voice):\n" +
		`A question or ordinary conversation: {"action":"none","edits":` + empty + `,"reply":"A brief answer."}`
	if state.Paused {
		return contract + "\nMotion is paused: only none with empty edits is permitted. The person must resume using the app."
	}
	if state.Running {
		edit := `{"speed_percent":25}`
		if state.MotionMode == MotionModeCreativeV2 {
			edit = "[" + edit + "]"
		} else {
			edit = `{"controls":` + edit + `}`
		}
		return contract + `
For a requested pace change to 25, only if 25 is inside saved limits: {"action":"update","edits":` + edit + `,"reply":"The requested pace is applied."}`
	}
	return contract + `
Only an explicit request to begin movement permits: {"action":"start","edits":` + empty + `,"reply":"Starting with the current settings."}
Changing settings while stopped is not permission to start. Otherwise use none with empty edits.`
}

// InteractiveResponseSchema is shared by production and prompt inspection.
func InteractiveResponseSchema(patterns []PatternChoice, capabilities Capabilities, state *MotionContext) json.RawMessage {
	if !capabilities.Motion || (capabilities.MotionMode != MotionModeCreativeV2 && capabilities.MotionMode != MotionModeLayered) {
		return PatternResponseSchema(patterns, capabilities, state)
	}
	current := MotionContext{MotionMode: capabilities.MotionMode}
	if state != nil {
		current = *state
	}
	_, limits := layeredContextScore(&current)
	schema := LayeredResponseSchema(limits, capabilities.MoodTracking)
	if capabilities.MotionMode == MotionModeCreativeV2 {
		schema = CreativeV2ResponseSchema(limits, capabilities.MoodTracking)
	}
	schema = withRecallSchema(schema, capabilities.MotionMode, len(recallableScores(current)))
	return continuousActionSchema(schema, current)
}
