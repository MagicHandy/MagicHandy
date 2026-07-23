package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion/semantic"
)

// AskActor streams roleplay dialogue aware of the current physical intent.
func AskActor(
	ctx context.Context,
	provider llm.Provider,
	model string,
	userMessage string,
	history []llm.Message,
	intent semantic.LLMIntent,
	onToken func(string) error,
) (string, error) {
	if provider == nil {
		return "", fmt.Errorf("LLM provider is required")
	}
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return "", fmt.Errorf("chat message is required")
	}

	prefix := fmt.Sprintf(
		"System: You are currently performing %s on the %s zone at intensity %d/10. Match dialogue pace to this physical state.",
		intent.Action, intent.Location, intent.Intensity,
	)
	messages := []llm.Message{{Role: "system", Content: prefix}}
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})

	request := llm.ChatRequest{
		Messages:    messages,
		Model:       model,
		Temperature: 0.7,
		MaxTokens:   256,
	}
	return provider.StreamChat(ctx, request, onToken)
}
