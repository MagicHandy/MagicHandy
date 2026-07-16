package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const llamaCPPProviderName = "llama_cpp"

// LlamaCPPProvider talks to llama-server through its OpenAI-compatible API.
type LlamaCPPProvider struct {
	baseURL string
	model   string
	client  *http.Client
	timeout time.Duration
}

// NewLlamaCPPProvider creates a llama.cpp HTTP provider.
func NewLlamaCPPProvider(options HTTPProviderOptions) (*LlamaCPPProvider, error) {
	normalized, err := normalizeHTTPOptions(options)
	if err != nil {
		return nil, err
	}
	return &LlamaCPPProvider{
		baseURL: normalized.BaseURL,
		model:   normalized.Model,
		client:  normalized.Client,
		timeout: normalized.Timeout,
	}, nil
}

// StreamChat streams chat text from llama.cpp's OpenAI-compatible endpoint.
func (p *LlamaCPPProvider) StreamChat(ctx context.Context, request ChatRequest, onDelta func(string) error) (string, error) {
	ctx, cancel := checkedRequestContext(ctx, p.timeout)
	defer cancel()

	body := openAIChatRequest{
		Model:                firstNonEmpty(request.Model, p.model),
		Messages:             request.Messages,
		Stream:               true,
		Temperature:          request.Temperature,
		MaxTokens:            request.MaxTokens,
		ThinkingBudgetTokens: request.ReasoningBudgetTokens,
		ResponseFormat: &openAIResponseFormat{
			Type: "json_object",
		},
	}
	if request.ReasoningMode == "off" {
		body.ChatTemplateKwargs = &openAIChatTemplateKwargs{EnableThinking: false}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode llama.cpp chat request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build llama.cpp chat request: %w", err)
	}
	httpRequest.Header.Set("Accept", "text/event-stream")
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("llama.cpp chat request failed: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("llama.cpp chat returned %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}

	return readOpenAIEventStream(response.Body, onDelta)
}

// Status checks the llama.cpp health endpoint without loading or downloading a model.
func (p *LlamaCPPProvider) Status(ctx context.Context) ProviderStatus {
	ctx, cancel := checkedRequestContext(ctx, 5*time.Second)
	defer cancel()

	status := ProviderStatus{
		Provider: llamaCPPProviderName,
		BaseURL:  p.baseURL,
		Model:    p.model,
		Loaded:   true,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/health", nil)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	response, err := p.client.Do(request)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status.Message = fmt.Sprintf("health endpoint returned %d", response.StatusCode)
		return status
	}

	models, err := p.listModels(ctx)
	if err != nil {
		status.Available = true
		status.Message = fmt.Sprintf("ready; selected model could not be verified because the model list is unavailable: %s", err)
		return status
	}
	status.Models = models
	status.ModelAvailable = modelListed(p.model, models)
	status.Available = status.ModelAvailable
	if !status.ModelAvailable {
		status.Message = fmt.Sprintf("model %q is not reported by llama.cpp", p.model)
		return status
	}
	status.Message = "ready"
	return status
}

func (p *LlamaCPPProvider) listModels(ctx context.Context) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("models endpoint returned %d", response.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	models := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if strings.TrimSpace(model.ID) != "" {
			models = append(models, strings.TrimSpace(model.ID))
		}
	}
	if len(models) == 0 {
		return nil, errors.New("models endpoint returned no models")
	}
	return models, nil
}

type openAIChatRequest struct {
	Model                string                    `json:"model"`
	Messages             []Message                 `json:"messages"`
	Stream               bool                      `json:"stream"`
	Temperature          float64                   `json:"temperature"`
	MaxTokens            int                       `json:"max_tokens,omitempty"`
	ThinkingBudgetTokens int                       `json:"thinking_budget_tokens,omitempty"`
	ChatTemplateKwargs   *openAIChatTemplateKwargs `json:"chat_template_kwargs,omitempty"`
	ResponseFormat       *openAIResponseFormat     `json:"response_format,omitempty"`
}

type openAIChatTemplateKwargs struct {
	EnableThinking bool `json:"enable_thinking"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func readOpenAIEventStream(body io.Reader, onDelta func(string) error) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	var builder strings.Builder
	finishReason := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			if finishReason == "length" {
				return builder.String(), ErrOutputTruncated
			}
			return builder.String(), nil
		}

		var chunk openAIChatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return builder.String(), fmt.Errorf("decode llama.cpp stream chunk: %w", err)
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return builder.String(), errors.New(chunk.Error.Message)
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
			delta := choice.Delta.Content
			if delta == "" {
				delta = choice.Message.Content
			}
			if delta == "" {
				continue
			}
			if err := appendStreamDelta(&builder, delta, onDelta); err != nil {
				return builder.String(), err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return builder.String(), fmt.Errorf("read llama.cpp stream: %w", err)
	}
	if finishReason == "length" {
		return builder.String(), ErrOutputTruncated
	}
	if finishReason != "" {
		return builder.String(), nil
	}
	return builder.String(), errIncompleteStream
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func modelListed(model string, models []string) bool {
	model = strings.TrimSpace(model)
	for _, candidate := range models {
		if strings.TrimSpace(candidate) == model {
			return true
		}
	}
	return false
}
