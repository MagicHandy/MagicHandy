package chat

import (
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const bandGuidePrefix = "Current score for this turn: outer band "

// The newest request can refer to a prior definition or to the movement that
// is actually playing. Keep the authority and history availability explicit
// beside that request, without displacing any of the available conversation.
func (s Service) withMotionHistoryGuide(input llm.ChatRequest, state MotionContext, caps Capabilities, promptID string) llm.ChatRequest {
	if s.TrustedMotionInput || !caps.LocalInference || !caps.Motion || caps.MotionMode != MotionModeLayered || promptLocaleForID(promptID) != promptLocaleEnglish {
		return input
	}
	last := len(input.Messages) - 1
	if last < 0 || input.Messages[last].Role != "user" || state.Layered == nil || state.Paused || state.Autopilot || state.ManualRestartAvailable {
		return input
	}
	if state.Outputs != nil && (state.Outputs.Strokers == 0 || paceOnlyOnly(state.Outputs)) {
		return input
	}
	score := state.Layered
	var guide string
	switch {
	case caps.SimpleMotion && input.HistoryMessagesDropped > 0:
		guide = fmt.Sprintf(bandGuidePrefix+`%d..%d, overall speed %d. %d earlier messages are outside this request. A named pattern can be applied only if its actual settings appear in the available dialogue or an identified saved score. If its definition is missing, use {"action":"none","edits":{}} and ask for its settings. An independent instruction about the motion playing now still applies. For a relative pace edit, put the signed change requested this turn directly in change_by.speed_percent and omit every other control.`, score.MinPercent, score.MaxPercent, score.SpeedPercent, input.HistoryMessagesDropped)
	case caps.AdvancedMotion && includedDialogueHistory(input.Messages) > PromptHistoryLimit && state.Running:
		guide = `Use the latest user message to decide whether to speak or act. Earlier definitions and corrections are reference material. For a read-back or question about an earlier pattern, return {"action":"none","edits":{},"reply":"..."} with the requested facts only in the reply; the current pattern keeps playing. When the user instead asks to APPLY a named pattern, encode the latest corrected values in edits. For example, applying a full sweep from 12 to 72 at speed 28 while running uses {"action":"update","edits":{"composition":"full_band","controls":{"max_percent":{"op":"set","value":72},"min_percent":{"op":"set","value":12},"speed_percent":{"op":"set","value":28}}},"reply":"Applying the requested sweep."} Use the actual requested settings. Overall speed is separate from a phrase group's relative rate.`
	default:
		return input
	}
	return withOptionalTurnGuide(input, guide, s.PromptBudget.ContextSize)
}

func withOptionalTurnGuide(input llm.ChatRequest, guide string, contextSize int) llm.ChatRequest {
	last := len(input.Messages) - 1
	candidate := input
	candidate.Messages = make([]llm.Message, 0, len(input.Messages)+1)
	candidate.Messages = append(candidate.Messages, input.Messages[:last]...)
	candidate.Messages = append(candidate.Messages, llm.Message{Role: "system", Content: guide}, input.Messages[last])
	_, budget, err := llm.BudgetChatRequest(candidate, contextSize)
	if err != nil || budget.HistoryMessagesDropped > input.HistoryMessagesDropped {
		return input
	}
	return candidate
}
