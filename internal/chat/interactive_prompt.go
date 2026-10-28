package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/config"
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
			if capabilities.MotionMode == MotionModeLayered && capabilities.IntermediateMotion && !capabilities.AdvancedMotion {
				section.Text = intermediateLayeredContract(section.Text)
			}
			if capabilities.MotionMode == MotionModeLayered && capabilities.AdvancedMotion {
				// Include the state-specific examples generated above. Only this
				// code-owned contract is translated, never conversation text.
				section.Text = hostedLayeredContract(section.Text)
			}
			section.Characters = utf8.RuneCountInString(section.Text)
			section.Bytes = len(section.Text)
		}
		if section.ID == "output_guard" {
			sections = appendPromptSection(sections, "live_action", "Live chat action", continuousActionGuideFor(current))
			if !capabilities.AdvancedMotion && capabilities.MotionMode == MotionModeLayered && state != nil && !current.Running && !current.Paused {
				sections = appendPromptSection(sections, "local_start_intent", "Stopped motion intent", localStoppedIntentGuide)
			}
		}
		sections = append(sections, section)
	}
	if guide := simpleInteractiveGuideForLocale(capabilities, current, promptLocaleForID(set.ID)); guide != "" {
		sections = appendPromptSection(sections, "simple_edit_check", "Simple motion edits", guide)
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

// Local stopped chat needs an explicit distinction between a performance
// request and an explanation. Keep it off active motion and autonomous prompts
// so their established small-model budget and timing are unchanged.
const localStoppedIntentGuide = `A request to perform a rhythm asks you to start it now, without another confirmation. An explanation, quoted command or future possibility asks for conversation, not a demonstration. "Give me a rolling rhythm" requests motion; "What would it feel like?" does not. Decide the action before writing the reply.`

// Experimental examples omit action. Replaying those beside a mandatory live
// action field encourages unconstrained models to copy an invalid envelope.
// Use state-valid examples without inferring authority from a returned edit.
func continuousInteractiveContract(contract string, state MotionContext) string {
	contract = continuousInteractiveExamples(contract, state)
	if state.Autopilot {
		// The constrained decoder emits required top-level fields in schema
		// order: action, edits, reply, stay_unchanged. Do not demonstrate a hold
		// field before reply. Preserve nested edit order and skip prose/templates.
		lines := strings.Split(contract, "\n")
		for i, line := range lines {
			start := strings.Index(line, `{"action":`)
			if start < 0 {
				continue
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal([]byte(line[start:]), &fields) != nil || fields["reply"] == nil {
				continue
			}
			fields["stay_unchanged"] = json.RawMessage(`false`)
			// This code-owned example explicitly asks for a lasting hold.
			// It must not teach the opposite decision; no user text is matched.
			if (state.StandingHold && string(fields["action"]) == `"none"`) || string(fields["reply"]) == `"Keeping this exact score."` {
				fields["stay_unchanged"] = json.RawMessage(`true`)
			}
			encoded, _ := json.Marshal(fields)
			lines[i] = line[:start] + string(encoded)
		}
		contract = strings.Join(lines, "\n")
	}
	return contract
}

func continuousInteractiveExamples(contract string, state MotionContext) string {
	if state.Running && !state.Paused && !unavailableVibrationOnly(state) {
		// Keep the established motion examples for active local sessions. Only
		// fix their envelope; removing useful geometry examples harms smaller models.
		contract = strings.ReplaceAll(contract, `{"edits":[]`, `{"action":"none","edits":[]`)
		contract = strings.ReplaceAll(contract, `{"edits":{}`, `{"action":"none","edits":{}`)
		return strings.ReplaceAll(contract, `{"edits":`, `{"action":"update","edits":`)
	}
	_, limits := layeredContextScore(&state)
	speed := min(max(20, limits.SpeedMinPercent), limits.SpeedMaxPercent)
	if state.MotionMode == MotionModeLayered {
		contract = strings.ReplaceAll(contract, `Output {"edits":[...],"reply":"..."}.`, `Output {"action":"none|start|update","edits":[...],"reply":"..."}; choose one action permitted by the current state.`)
		if begin := strings.Index(contract, "\nExamples:"); begin >= 0 {
			if end := strings.Index(contract[begin:], "Independent refinements"); end >= 0 {
				contract = contract[:begin] + "\n\n" + contract[begin+end:]
			}
		}
		contract = strings.ReplaceAll(contract, `Output {"edits":{...},"reply":"..."}.`, `Output {"action":"none|start|update","edits":{...},"reply":"..."}; choose one action permitted by the current state.`)
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
	if unavailableVibrationOnly(state) {
		return contract + "\nThe only selected output is not ready or has zero user intensity. Only none with empty edits is permitted. Explain the backend state and the needed connection or intensity setting."
	}
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
	if state.MotionMode == MotionModeLayered {
		// Keep a state-valid geometry example when stopped. The generic empty
		// start example alone taught smaller models to keep the default band
		// even when the human explicitly supplied different endpoints.
		contract += fmt.Sprintf(`
For "Start full strokes from 0 to 100 at speed %d" (current band 5..95): {"action":"start","edits":{"controls":{"speed_percent":%d},"geometry":"full_slider"},"reply":"Starting full strokes between 0 and 100 at %d percent."}`, speed, speed, speed)
	} else {
		contract += fmt.Sprintf(`
For "Start full strokes from 0 to 100 at speed %d" (current band 5..95): {"action":"start","edits":[{"range":{"max_percent":100,"min_percent":0}},{"focus":{"mix_percent":0,"position_percent":50,"roam_percent":100,"width_percent":25}},{"speed_percent":%d}],"reply":"Starting full strokes between 0 and 100."}`, speed, speed)
	}
	return contract + `
For "Use the current motion again": {"action":"start","edits":` + empty + `,"reply":"Starting with the current settings."}
Apply requested edits with start. A fresh motion request can start after an earlier Stop; questions and explicitly future/settings-only requests use none.`
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
	schema := continuousEditSchema(limits, capabilities, current)
	return withVibrationLevelSchema(withVibrationSchema(continuousActionSchema(schema, current), capabilities, &current), capabilities, &current)
}

// Both interactive and autonomous requests use the same tier's edit grammar;
// their lifecycle and timing envelopes remain independently constrained.
func continuousEditSchema(limits config.MotionSettings, capabilities Capabilities, state MotionContext) json.RawMessage {
	schema := layeredResponseSchema(limits, capabilities.MoodTracking, capabilities.AdvancedMotion)
	if capabilities.MotionMode == MotionModeCreativeV2 {
		schema = CreativeV2ResponseSchema(limits, capabilities.MoodTracking)
	}
	schema = withRecallSchema(schema, capabilities.MotionMode, len(recallableScores(state)))
	if capabilities.MotionMode == MotionModeLayered && capabilities.IntermediateMotion && !capabilities.AdvancedMotion {
		schema = intermediateLayeredSchema(schema)
	}
	return schema
}
