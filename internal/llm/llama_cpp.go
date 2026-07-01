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
		Model:       firstNonEmpty(request.Model, p.model),
		Messages:    request.Messages,
		Stream:      true,
		Temperature: request.Temperature,
		ResponseFormat: &openAIResponseFormat{
			Type: "json_object",
		},
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

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/health", nil)
	if err != nil {
		return ProviderStatus{Provider: llamaCPPProviderName, BaseURL: p.baseURL, Model: p.model, Message: err.Error()}
	}
	response, err := p.client.Do(request)
	if err != nil {
		return ProviderStatus{Provider: llamaCPPProviderName, BaseURL: p.baseURL, Model: p.model, Message: err.Error()}
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ProviderStatus{
			Provider: llamaCPPProviderName,
			BaseURL:  p.baseURL,
			Model:    p.model,
			Message:  fmt.Sprintf("health endpoint returned %d", response.StatusCode),
		}
	}
	return ProviderStatus{
		Provider:  llamaCPPProviderName,
		BaseURL:   p.baseURL,
		Model:     p.model,
		Available: true,
		Message:   "ready",
	}
}

type openAIChatRequest struct {
	Model          string                `json:"model"`
	Messages       []Message             `json:"messages"`
	Stream         bool                  `json:"stream"`
	Temperature    float64               `json:"temperature,omitempty"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
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
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func readOpenAIEventStream(body io.Reader, onDelta func(string) error) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	var builder strings.Builder
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
			delta := choice.Delta.Content
			if delta == "" {
				delta = choice.Message.Content
			}
			if delta == "" {
				continue
			}
			builder.WriteString(delta)
			if onDelta != nil {
				if err := onDelta(delta); err != nil {
					return builder.String(), err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return builder.String(), fmt.Errorf("read llama.cpp stream: %w", err)
	}
	return builder.String(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
