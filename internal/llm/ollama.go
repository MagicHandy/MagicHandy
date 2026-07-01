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

const ollamaProviderName = "ollama"

// OllamaProvider talks to an externally managed Ollama daemon.
type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
	timeout time.Duration
}

// NewOllamaProvider creates an Ollama HTTP provider.
func NewOllamaProvider(options HTTPProviderOptions) (*OllamaProvider, error) {
	normalized, err := normalizeHTTPOptions(options)
	if err != nil {
		return nil, err
	}
	return &OllamaProvider{
		baseURL: normalized.BaseURL,
		model:   normalized.Model,
		client:  normalized.Client,
		timeout: normalized.Timeout,
	}, nil
}

// StreamChat streams chat text from Ollama's /api/chat endpoint.
func (p *OllamaProvider) StreamChat(ctx context.Context, request ChatRequest, onDelta func(string) error) (string, error) {
	ctx, cancel := checkedRequestContext(ctx, p.timeout)
	defer cancel()

	body := ollamaChatRequest{
		Model:    firstNonEmpty(request.Model, p.model),
		Messages: request.Messages,
		Stream:   true,
		Format:   "json",
		Options: map[string]any{
			"temperature": request.Temperature,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode Ollama chat request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build Ollama chat request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/x-ndjson")
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("Ollama chat request failed: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("Ollama chat returned %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}

	return readOllamaStream(response.Body, onDelta)
}

// Status checks Ollama daemon reachability without loading or downloading a model.
func (p *OllamaProvider) Status(ctx context.Context) ProviderStatus {
	ctx, cancel := checkedRequestContext(ctx, 5*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/tags", nil)
	if err != nil {
		return ProviderStatus{Provider: ollamaProviderName, BaseURL: p.baseURL, Model: p.model, Message: err.Error()}
	}
	response, err := p.client.Do(request)
	if err != nil {
		return ProviderStatus{Provider: ollamaProviderName, BaseURL: p.baseURL, Model: p.model, Message: err.Error()}
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ProviderStatus{
			Provider: ollamaProviderName,
			BaseURL:  p.baseURL,
			Model:    p.model,
			Message:  fmt.Sprintf("tags endpoint returned %d", response.StatusCode),
		}
	}
	return ProviderStatus{
		Provider:  ollamaProviderName,
		BaseURL:   p.baseURL,
		Model:     p.model,
		Available: true,
		Message:   "ready",
	}
}

type ollamaChatRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Format   string         `json:"format"`
	Options  map[string]any `json:"options,omitempty"`
}

type ollamaChatChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Error string `json:"error,omitempty"`
}

func readOllamaStream(body io.Reader, onDelta func(string) error) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	var builder strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk ollamaChatChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return builder.String(), fmt.Errorf("decode Ollama stream chunk: %w", err)
		}
		if chunk.Error != "" {
			return builder.String(), errors.New(chunk.Error)
		}
		if chunk.Message.Content != "" {
			builder.WriteString(chunk.Message.Content)
			if onDelta != nil {
				if err := onDelta(chunk.Message.Content); err != nil {
					return builder.String(), err
				}
			}
		}
		if chunk.Done {
			return builder.String(), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return builder.String(), fmt.Errorf("read Ollama stream: %w", err)
	}
	return builder.String(), nil
}
