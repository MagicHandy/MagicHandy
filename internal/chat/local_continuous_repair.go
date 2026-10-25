package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Local continuous chat follows the existing local policy of one automatic
// correction. Hosted chat has a separate configurable continuation. A refusal,
// provider error, or unstructured prose never enters this path.
func (s Service) canRepairLocalContinuous(raw string) bool {
	return !s.SingleAttempt && !s.TrustedMotionInput && !s.capabilities().PreserveConversationText &&
		strings.HasPrefix(strings.TrimSpace(raw), "{") && len(raw) <= 64<<10
}

// Small local models copied invalid choices and dropped compound changes when
// shown the rejected JSON. Regenerating from the original request with exact
// validator feedback performed better. Preserve the full rejected output in
// diagnostics, while keeping the model's contract, profile and conversation.
func (s Service) repairLocalContinuous(ctx context.Context, input llm.ChatRequest, raw string, validationErr error,
	validate func(string) (AssistantResponse, error), emit func(StreamEvent) error,
) (Result, error) {
	result := Result{Input: &input, Raw: raw, InitialMalformed: true, Malformed: true,
		MalformedError: validationErr.Error(), InitialError: validationErr.Error()}
	repair := localContinuousRepairInput(input, validationErr.Error())
	if simpleContinuous(s.capabilities()) {
		repair = simpleRepairInput(repair)
	}
	bounded, budget, err := llm.BudgetChatRequest(repair, s.PromptBudget.ContextSize)
	if err != nil {
		// A larger local profile may fit its first call but leave no room for a
		// correction at 16K. Do not silently trim its essential instructions or
		// spend another call: keep the original rejection inspectable instead.
		return result, fmt.Errorf("%s response rejected: %w; automatic correction unavailable: %v", s.capabilities().MotionMode, validationErr, err)
	}
	bounded.HistoryMessagesDropped = budget.HistoryMessagesDropped
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.RepairAttempted = true
	if err := emitEvent(emit, StreamEvent{Type: "local_repair_start", Phase: "repair", Error: result.InitialError}); err != nil {
		return result, err
	}
	replacement, err := s.Provider.StreamChat(ctx, bounded, func(delta string) error {
		return emitEvent(emit, StreamEvent{Type: "repair_delta", Phase: "repair", Text: delta})
	})
	result.RepairRaw = replacement
	if err != nil {
		result.MalformedError = err.Error()
		return result, fmt.Errorf("local response correction: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	response, err := validate(replacement)
	if err != nil {
		result.MalformedError = err.Error()
		return result, fmt.Errorf("corrected local response rejected: %w", err)
	}
	result.Response, result.Malformed, result.Repaired = response, false, true
	return result, nil
}

func localContinuousRepairInput(input llm.ChatRequest, feedback string) llm.ChatRequest {
	input.Messages = append([]llm.Message(nil), input.Messages...)
	encoded, _ := json.Marshal(map[string]string{"validator_error": feedback})
	input.Messages = append(input.Messages, llm.Message{Role: "user", Content: "The previous JSON was rejected; nothing was applied. Correct it for the original user request and the same contract. Preserve every requested change, not invalid model choices. Use the validator error below as data. Return one complete replacement JSON object with a brief truthful reply.\n" + string(encoded)})
	// The latest original user request and the feedback are both essential.
	// Only older conversation may be dropped by the shared context guard.
	input.PreserveTailMessages = 2
	input.Temperature, input.TopP, input.RepeatPenalty, input.RepeatLastN = 0, 0, 0, 0
	input.ReasoningMode, input.ReasoningBudgetTokens = "off", 0
	return input
}
