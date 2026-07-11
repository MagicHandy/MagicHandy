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
		return "", fmt.Errorf("ollama chat request failed: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("ollama chat returned %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}

	return readOllamaStream(response.Body, onDelta)
}

// Status checks Ollama daemon reachability without loading or downloading a model.
func (p *OllamaProvider) Status(ctx context.Context) ProviderStatus {
	status := ProviderStatus{
		Provider: ollamaProviderName,
		BaseURL:  p.baseURL,
		Model:    p.model,
		Loaded:   true,
	}
	installed, err := ListOllamaModels(ctx, p.baseURL, p.client)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	models := make([]string, 0, len(installed))
	for _, model := range installed {
		models = append(models, model.Name)
	}
	status.Models = models
	status.ModelAvailable = modelListed(p.model, models)
	status.Available = status.ModelAvailable
	if !status.ModelAvailable {
		status.Message = fmt.Sprintf("model %q is not installed in Ollama", p.model)
		return status
	}
	status.Message = "ready"
	return status
}

// OllamaModelInfo is one model reported by Ollama's non-loading tags endpoint.
type OllamaModelInfo struct {
	Name          string `json:"name"`
	Model         string `json:"model,omitempty"`
	ModifiedAt    string `json:"modified_at,omitempty"`
	SizeBytes     int64  `json:"size_bytes"`
	Digest        string `json:"digest,omitempty"`
	Format        string `json:"format,omitempty"`
	Family        string `json:"family,omitempty"`
	ParameterSize string `json:"parameter_size,omitempty"`
	Quantization  string `json:"quantization,omitempty"`
}

// ListOllamaModels lists daemon-managed models without requiring a selected
// model and without loading or downloading model data.
func ListOllamaModels(ctx context.Context, baseURL string, client *http.Client) ([]OllamaModelInfo, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("base URL for Ollama is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	ctx, cancel := checkedRequestContext(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("build Ollama model-list request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("list Ollama models: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("tags endpoint for Ollama returned %d", response.StatusCode)
	}
	return decodeOllamaModelInfo(response.Body)
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

func decodeOllamaModelInfo(body io.Reader) ([]OllamaModelInfo, error) {
	var payload struct {
		Models []struct {
			Name       string `json:"name"`
			Model      string `json:"model"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
			Digest     string `json:"digest"`
			Details    struct {
				Format            string `json:"format"`
				Family            string `json:"family"`
				ParameterSize     string `json:"parameter_size"`
				QuantizationLevel string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1024*1024)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Ollama tags response: %w", err)
	}
	models := make([]OllamaModelInfo, 0, len(payload.Models))
	for _, model := range payload.Models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			name = strings.TrimSpace(model.Model)
		}
		if name != "" {
			models = append(models, OllamaModelInfo{
				Name: name, Model: strings.TrimSpace(model.Model), ModifiedAt: model.ModifiedAt,
				SizeBytes: model.Size, Digest: model.Digest, Format: model.Details.Format,
				Family: model.Details.Family, ParameterSize: model.Details.ParameterSize,
				Quantization: model.Details.QuantizationLevel,
			})
		}
	}
	return models, nil
}
