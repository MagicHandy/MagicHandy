package llm

import "fmt"

// PromptBudget reports byte accounting, never an exact tokenizer count. One
// UTF-8 byte per input token is a deliberately conservative admission estimate
// for byte-fallback tokenizers. Remote servers may have smaller contexts.
type PromptBudget struct {
	InputBytes             int `json:"input_bytes"`
	LimitBytes             int `json:"limit_bytes"`
	HistoryMessagesDropped int `json:"history_messages_dropped"`
}

// BudgetChatRequest preserves complete system messages and the final exchange
// (including a repair's original user turn). Only oldest history is removed.
// An oversized essential prompt fails clearly instead of losing control rules.
func BudgetChatRequest(request ChatRequest, contextSize int) (ChatRequest, PromptBudget, error) {
	// Reserve output, bounded thinking, template/schema overhead, and room for
	// one repair exchange. No model-specific tokenizer dependency enters the core.
	limit := PromptInputByteLimit(contextSize, request.MaxTokens, request.ReasoningBudgetTokens, len(request.JSONSchema))
	budget := PromptBudget{LimitBytes: max(0, limit)}
	for _, message := range request.Messages {
		budget.InputBytes += len(message.Content) + 64
	}
	if budget.InputBytes <= limit {
		return request, budget, nil
	}
	retained := make([]Message, 0, len(request.Messages))
	tail := max(1, request.PreserveTailMessages)
	for i, message := range request.Messages {
		if budget.InputBytes > limit && message.Role != "system" && i < len(request.Messages)-tail {
			budget.InputBytes -= len(message.Content) + 64
			budget.HistoryMessagesDropped++
			continue
		}
		retained = append(retained, message)
	}
	if budget.InputBytes > limit {
		return request, budget, fmt.Errorf("LLM prompt exceeds the conservative input budget (%d bytes, limit %d after response reserves); shorten the profile or request, or increase the model context", budget.InputBytes, budget.LimitBytes)
	}
	request.Messages = retained
	return request, budget, nil
}

// PromptInputByteLimit is shared with composition so optional memories are
// selected within the same conservative context policy as provider admission.
func PromptInputByteLimit(contextSize, outputTokens, reasoningTokens, schemaBytes int) int {
	if contextSize <= 0 {
		contextSize = 32768
	}
	return max(0, contextSize-max(outputTokens, 256)-max(reasoningTokens, 0)-2048-max(schemaBytes, 0))
}
