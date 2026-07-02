package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const (
	maxUserMessageBytes = 4096
	maxHistoryMessages  = 12
)

// StreamEvent describes chat orchestration progress.
type StreamEvent struct {
	Type  string
	Phase string
	Text  string
	Error string
}

// Request is one local chat turn.
type Request struct {
	Message string
	History []llm.Message
}

// Result is the validated chat turn outcome.
type Result struct {
	Response         AssistantResponse
	Raw              string
	RepairRaw        string
	InitialMalformed bool
	Malformed        bool
	MalformedError   string
	Repaired         bool
}

// Service runs chat prompts, strict validation, and repair over an LLM provider.
type Service struct {
	Provider    llm.Provider
	PromptSetID string
	Model       string
}

// Complete streams a model response, repairs malformed JSON once, and returns a validated result.
func (s Service) Complete(ctx context.Context, request Request, emit func(StreamEvent) error) (Result, error) {
	if s.Provider == nil {
		return Result{}, errors.New("LLM provider is required")
	}
	userMessage := strings.TrimSpace(request.Message)
	if userMessage == "" {
		return Result{}, errors.New("chat message is required")
	}
	if len(userMessage) > maxUserMessageBytes {
		return Result{}, fmt.Errorf("chat message must be at most %d bytes", maxUserMessageBytes)
	}

	prompt, ok := PromptSetByID(s.PromptSetID)
	if !ok {
		return Result{}, fmt.Errorf("unknown prompt set %q", s.PromptSetID)
	}

	messages := buildMessages(prompt, request.History, userMessage)
	raw, err := s.Provider.StreamChat(ctx, llm.ChatRequest{
		Messages:    messages,
		Model:       s.Model,
		Temperature: 0.2,
	}, func(text string) error {
		return emitEvent(emit, StreamEvent{Type: "delta", Phase: "initial", Text: text})
	})
	if err != nil {
		return Result{}, err
	}

	response, parseErr := ParseAssistantResponse(raw)
	if parseErr == nil {
		return Result{Response: response, Raw: raw}, nil
	}

	result := Result{
		Raw:              raw,
		InitialMalformed: true,
		Malformed:        true,
		MalformedError:   parseErr.Error(),
	}
	if err := emitEvent(emit, StreamEvent{Type: "malformed", Phase: "initial", Error: parseErr.Error()}); err != nil {
		return result, err
	}

	repairMessages := []llm.Message{
		{Role: "system", Content: prompt.System},
		{Role: "user", Content: RepairPrompt(prompt, raw, parseErr.Error())},
	}
	repairRaw, repairErr := s.Provider.StreamChat(ctx, llm.ChatRequest{
		Messages:    repairMessages,
		Model:       s.Model,
		Temperature: 0,
	}, func(text string) error {
		return emitEvent(emit, StreamEvent{Type: "repair_delta", Phase: "repair", Text: text})
	})
	result.RepairRaw = repairRaw
	if repairErr != nil {
		result.MalformedError = repairErr.Error()
		return result, nil
	}

	repaired, repairParseErr := ParseAssistantResponse(repairRaw)
	if repairParseErr != nil {
		result.MalformedError = repairParseErr.Error()
		return result, nil
	}

	result.Response = repaired
	result.Malformed = false
	result.Repaired = true
	return result, nil
}

func buildMessages(prompt PromptSet, history []llm.Message, userMessage string) []llm.Message {
	messages := []llm.Message{{Role: "system", Content: prompt.System}}
	messages = append(messages, sanitizeHistory(history)...)
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})
	return messages
}

func sanitizeHistory(history []llm.Message) []llm.Message {
	if len(history) > maxHistoryMessages {
		history = history[len(history)-maxHistoryMessages:]
	}
	messages := make([]llm.Message, 0, len(history))
	for _, message := range history {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if len(content) > maxUserMessageBytes {
			content = content[:maxUserMessageBytes]
		}
		if role == "assistant" {
			content = assistantHistoryContent(content)
		}
		messages = append(messages, llm.Message{Role: role, Content: content})
	}
	return messages
}

func assistantHistoryContent(content string) string {
	if _, err := ParseAssistantResponse(content); err == nil {
		return content
	}
	response := AssistantResponse{
		Reply: content,
		Motion: &MotionCommand{
			Action: MotionActionNone,
		},
	}
	data, err := json.Marshal(response)
	if err != nil {
		return `{"reply":"Previous assistant reply omitted.","motion":{"action":"none"}}`
	}
	return string(data)
}

func emitEvent(emit func(StreamEvent) error, event StreamEvent) error {
	if emit == nil {
		return nil
	}
	return emit(event)
}
