package chat

import (
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const bandGuidePrefix = "Current score for this turn: outer band "

func ordinaryPositionChat(state MotionContext) bool {
	if state.Paused || state.Autopilot || state.ManualRestartAvailable {
		return false
	}
	return state.Outputs == nil || (state.Outputs.Strokers > 0 && !paceOnlyOnly(state.Outputs))
}

// Keep the local tier's edit shape and history availability beside the human
// request. Optional guidance must fit with the same admitted dialogue.
func (s Service) withLocalMotionGuide(input llm.ChatRequest, state MotionContext, caps Capabilities, promptID string) llm.ChatRequest {
	if s.TrustedMotionInput || !caps.LocalInference || !caps.Motion || !ordinaryPositionChat(state) || promptLocaleForID(promptID) != promptLocaleEnglish {
		return input
	}
	if state.Layered == nil {
		return input
	}
	contextSize := s.PromptBudget.ContextSize
	_, admitted, err := llm.BudgetChatRequest(input, contextSize)
	if err != nil {
		return input
	}
	score := state.Layered
	localTier := caps.SimpleMotion || caps.IntermediateMotion
	var guide string
	switch {
	case caps.MotionMode == MotionModeLayered && caps.SimpleMotion && admitted.HistoryMessagesDropped > 0:
		guide = fmt.Sprintf(bandGuidePrefix+`%d..%d, overall speed %d. %d earlier messages are outside this request. A named pattern can be applied only if its actual settings appear in the available dialogue or an identified saved score. If its definition is missing, use {"action":"none","edits":[]} and ask for its settings. When the corrected definition IS visible, apply both marks in a range group, full_band in a geometry group and overall speed in a controls group. An independent instruction about the motion playing now still applies. For a relative pace edit, put the signed change requested this turn directly in change_by.speed_percent and omit every other control.`, score.MinPercent, score.MaxPercent, score.SpeedPercent, admitted.HistoryMessagesDropped)
	case caps.MotionMode == MotionModeLayered && caps.AdvancedMotion && includedDialogueHistory(input.Messages) > PromptHistoryLimit && state.Running:
		guide = `Use the latest user message to decide whether to speak or act. Earlier definitions and corrections are reference material. For a read-back or question about an earlier pattern, return {"action":"none","edits":{},"reply":"..."} with the requested facts only in the reply; the current pattern keeps playing. When the user instead asks to APPLY a named pattern, encode the latest corrected values in edits. For example, applying a full sweep from 12 to 72 at speed 28 while running uses {"action":"update","edits":{"composition":"full_band","controls":{"max_percent":{"op":"set","value":72},"min_percent":{"op":"set","value":12},"speed_percent":{"op":"set","value":28}}},"reply":"Applying the requested sweep."} Use the actual requested settings. Overall speed is separate from a phrase group's relative rate.`
	case localTier:
		guide = localRequestedMotionGuide(caps, state)
		if guide == "" {
			return input
		}
	default:
		return input
	}
	compact := ""
	if caps.MotionMode == MotionModeLayered && localTier {
		compact = compactBandGuide
	}
	return withOptionalTurnGuide(input, guide, compact, contextSize, admitted.HistoryMessagesDropped)
}

const compactBandGuide = "Notes/queries: none. Apply: both marks, full_band, speed. Missing: ask."

func withOptionalTurnGuide(input llm.ChatRequest, guide, compact string, contextSize, admittedDrops int) llm.ChatRequest {
	last := len(input.Messages) - 1
	if last < 0 || input.Messages[last].Role != "user" {
		return input
	}
	candidate := input
	candidate.Messages = make([]llm.Message, 0, len(input.Messages)+1)
	candidate.Messages = append(candidate.Messages, input.Messages[:last]...)
	candidate.Messages = append(candidate.Messages, llm.Message{Role: "system"}, input.Messages[last])
	for _, text := range []string{guide, compact} {
		if text == "" {
			break
		}
		candidate.Messages[last].Content = text
		_, budget, err := llm.BudgetChatRequest(candidate, contextSize)
		if err == nil && budget.HistoryMessagesDropped == admittedDrops {
			return candidate
		}
	}
	return input
}
