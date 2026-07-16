package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const (
	maxUserMessageBytes = 4096
	maxHistoryMessages  = 12
	emptyRepairContext  = `{"_malformed":"empty_or_truncated_output"}`
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
// Prompt is the resolved behavior profile; Memories are the enabled memory
// texts (empty when the memory switch is off — chat must work without them).
type Service struct {
	Provider              llm.Provider
	Prompt                PromptSet
	Model                 string
	MaxTokens             int
	ReasoningMode         string
	ReasoningBudgetTokens int
	Memories              []string
	Patterns              []PatternChoice
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

	prompt := s.Prompt
	if strings.TrimSpace(prompt.ID) == "" {
		prompt, _ = BuiltinPromptSetByID(DefaultPromptSetID)
	}
	systemPrompt := ComposeSystemWithPatterns(prompt, s.Memories, s.Patterns)

	messages := buildMessages(systemPrompt, request.History, userMessage)
	raw, err := s.Provider.StreamChat(ctx, llm.ChatRequest{
		Messages:              messages,
		Model:                 s.Model,
		Temperature:           0.2,
		MaxTokens:             s.MaxTokens,
		ReasoningMode:         s.ReasoningMode,
		ReasoningBudgetTokens: s.ReasoningBudgetTokens,
	}, func(text string) error {
		return emitEvent(emit, StreamEvent{Type: "delta", Phase: "initial", Text: text})
	})
	truncated := errors.Is(err, llm.ErrOutputTruncated)
	if err != nil && !truncated {
		return Result{}, err
	}

	response, parseErr := ParseAssistantResponseWithPatterns(raw, s.Patterns)
	if parseErr == nil {
		return Result{Response: response, Raw: raw}, nil
	}
	if truncated {
		parseErr = fmt.Errorf("assistant response was truncated before valid JSON: %w", parseErr)
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

	repairMessages := append([]llm.Message(nil), messages...)
	repairContext := strings.TrimSpace(raw)
	if repairContext == "" {
		repairContext = emptyRepairContext
	}
	repairMessages = append(repairMessages, llm.Message{Role: "assistant", Content: repairContext})
	repairMessages = append(repairMessages, llm.Message{Role: "user", Content: RepairPrompt(prompt, parseErr.Error())})
	repairRaw, repairErr := s.Provider.StreamChat(ctx, llm.ChatRequest{
		Messages:      repairMessages,
		Model:         s.Model,
		Temperature:   0,
		MaxTokens:     s.MaxTokens,
		ReasoningMode: "off",
	}, func(text string) error {
		return emitEvent(emit, StreamEvent{Type: "repair_delta", Phase: "repair", Text: text})
	})
	result.RepairRaw = repairRaw
	repairTruncated := errors.Is(repairErr, llm.ErrOutputTruncated)
	if repairErr != nil && !repairTruncated {
		result.MalformedError = repairErr.Error()
		return result, fmt.Errorf("repair assistant response: %w", repairErr)
	}

	repaired, repairParseErr := ParseAssistantResponseWithPatterns(repairRaw, s.Patterns)
	if repairParseErr != nil {
		if repairTruncated {
			repairParseErr = fmt.Errorf("repaired response was truncated before valid JSON: %w", repairParseErr)
		}
		result.MalformedError = repairParseErr.Error()
		return result, nil
	}

	result.Response = repaired
	result.Malformed = false
	result.Repaired = true
	return result, nil
}

func buildMessages(systemPrompt string, history []llm.Message, userMessage string) []llm.Message {
	messages := []llm.Message{{Role: "system", Content: systemPrompt}}
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
			content = truncateUTF8Bytes(content, maxUserMessageBytes)
		}
		if role == "assistant" {
			content = assistantHistoryContent(content)
		}
		messages = append(messages, llm.Message{Role: role, Content: content})
	}
	return messages
}

func truncateUTF8Bytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && end < len(value) && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func assistantHistoryContent(content string) string {
	var candidate AssistantResponse
	_ = json.Unmarshal([]byte(content), &candidate)
	choices := defaultPatternChoices()
	if candidate.Motion != nil && strings.TrimSpace(candidate.Motion.PatternID) != "" {
		choices = append(choices, PatternChoice{ID: candidate.Motion.PatternID})
	}
	if _, err := parseAssistantResponse(content, choices, false); err == nil {
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
