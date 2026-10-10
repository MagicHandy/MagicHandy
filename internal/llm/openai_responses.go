package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// TokenSource returns the credential for one captured account registration.
// Implementations refresh host-side; credentials never enter request settings.
type TokenSource func(context.Context) (string, error)

// OpenAIOptions is separate from the local runtime's sampling/health options.
type OpenAIOptions struct {
	// PlanUsage enables SIWC's preview field restrictions. API-key Responses
	// keeps its separate parameter capabilities and billing authorization.
	PlanUsage       bool
	Model           string
	ReasoningEffort string
	Token           TokenSource
	Client          *http.Client
	Timeout         time.Duration
	// BaseURL is injectable for protocol tests; the app uses the public API.
	BaseURL string
}

// OpenAIResponsesProvider implements ChatGPT plan inference through Responses.
type OpenAIResponsesProvider struct{ options OpenAIOptions }

// NewOpenAIResponsesProvider keeps ChatGPT plan and API-key protocols distinct.
func NewOpenAIResponsesProvider(options OpenAIOptions) (*OpenAIResponsesProvider, error) {
	if err := ValidateCloudModel(options.Model); err != nil {
		return nil, err
	}
	if options.Token == nil {
		return nil, errors.New("cloud credential source is required")
	}
	if options.BaseURL == "" {
		options.BaseURL = "https://api.openai.com/v1"
	}
	if options.Timeout <= 0 {
		options.Timeout = 90 * time.Second
	}
	if options.Client == nil {
		options.Client = &http.Client{Timeout: options.Timeout}
	}
	options.Client = noRedirectClient(options.Client)
	return &OpenAIResponsesProvider{options: options}, nil
}

// CloudModel is an account-visible model from the Responses provider catalog.
type CloudModel struct {
	Slug                     string           `json:"slug"`
	DisplayName              string           `json:"display_name"`
	Visibility               string           `json:"visibility"`
	DefaultReasoningLevel    string           `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels []ReasoningLevel `json:"supported_reasoning_levels,omitempty"`
}

// ReasoningLevel is account-specific catalog metadata, separate from local
// runtime reasoning switches and paid service tiers.
type ReasoningLevel struct {
	Effort string `json:"effort"`
}

// Models reads the authenticated provider catalog, preserving provider order.
func (p *OpenAIResponsesProvider) Models(ctx context.Context) ([]CloudModel, error) {
	response, err := p.request(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	var catalog struct {
		Models []CloudModel `json:"models"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, maxStreamResponseBytes)).Decode(&catalog) != nil {
		return nil, &CloudError{Kind: "unavailable"}
	}
	models := make([]CloudModel, 0, len(catalog.Models))
	if !p.options.PlanUsage {
		for _, entry := range catalog.Data {
			if ValidateCloudModel(entry.ID) == nil {
				models = append(models, CloudModel{Slug: entry.ID, DisplayName: entry.ID, Visibility: "list"})
			}
		}
	}
	for _, model := range catalog.Models {
		if model.Visibility == "list" && ValidateCloudModel(model.Slug) == nil {
			models = append(models, model)
		}
	}
	return models, nil
}

// Status reports model discovery, independently of real-generation readiness.
func (p *OpenAIResponsesProvider) Status(ctx context.Context) ProviderStatus {
	status := ProviderStatus{Provider: "chatgpt", BaseURL: p.options.BaseURL, Model: p.options.Model}
	if !p.options.PlanUsage {
		status.Provider = "openai"
	}
	models, err := p.Models(ctx)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	status.Available = true
	for _, model := range models {
		status.Models = append(status.Models, model.Slug)
		if model.Slug == p.options.Model {
			status.ModelAvailable = true
		}
	}
	return status
}

// StreamChat buffers Responses output until terminal completion is confirmed.
func (p *OpenAIResponsesProvider) StreamChat(ctx context.Context, request ChatRequest, onDelta func(string) error) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.options.Timeout)
	defer cancel()
	bounded, budget, err := BudgetChatRequest(request, 0)
	if request.OnBudget != nil {
		request.OnBudget(budget)
	}
	if err != nil {
		return "", err
	}
	request = bounded
	input := make([]Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role == "system" {
			message.Role = "developer"
		}
		input = append(input, message)
	}
	body := map[string]any{"model": p.options.Model, "input": input, "store": false, "stream": true}
	if p.options.ReasoningEffort != "" {
		body["reasoning"] = map[string]string{"effort": p.options.ReasoningEffort}
	}
	if !p.options.PlanUsage && request.MaxTokens > 0 {
		body["max_output_tokens"] = request.MaxTokens
	}
	if len(request.JSONSchema) > 0 {
		schema, err := OpenAISchema(request.JSONSchema)
		if err != nil {
			return "", err
		}
		body["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "motion_proposal", "strict": true, "schema": schema}}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	response, err := p.request(ctx, http.MethodPost, "/responses", encoded)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := consumeResponsesStream(response.Body, request.OnProgress)
	if err != nil {
		return "", err
	}
	if len(request.JSONSchema) > 0 {
		raw, err = DomainOutput(raw, request.JSONSchema)
	}
	if err != nil {
		return "", err
	}
	// Deliver only a completed proposal; a quota failure can follow text deltas.
	if onDelta != nil {
		if err := onDelta(raw); err != nil {
			return "", err
		}
	}
	return raw, nil
}

func (p *OpenAIResponsesProvider) request(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	token, err := p.options.Token(ctx)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.options.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, &CloudError{Kind: "unavailable"}
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream, application/json")
	response, err := p.options.Client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &CloudError{Kind: "unavailable"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer func() { _ = response.Body.Close() }()
		var failure struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&failure)
		return nil, cloudResponseError(response.StatusCode, failure.Error.Code)
	}
	return response, nil
}

type responsesEvent struct {
	Type     string `json:"type"`
	Delta    string `json:"delta"`
	Code     string `json:"code"`
	Response struct {
		Status string `json:"status"`
		Error  struct {
			Code string `json:"code"`
		} `json:"error"`
		Output []struct {
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		} `json:"output"`
	} `json:"response"`
}

func consumeResponsesStream(reader io.Reader, progress func(ProviderProgress)) (string, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 4*maxStreamResponseBytes))
	scanner.Buffer(make([]byte, 4096), maxStreamResponseBytes)
	var output strings.Builder
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		if line != "" || len(data) == 0 {
			continue
		}
		var event responsesEvent
		if json.Unmarshal([]byte(strings.Join(data, "\n")), &event) != nil {
			return "", &CloudError{Kind: "incomplete"}
		}
		data = nil
		if progress != nil {
			progress(ProviderProgress{Activity: true})
		}
		switch event.Type {
		case "response.output_text.delta":
			if err := appendStreamDelta(&output, event.Delta, nil); err != nil {
				return "", &CloudError{Kind: "incomplete"}
			}
		case "response.refusal.delta", "response.refusal.done":
			return "", &CloudError{Kind: "refusal"}
		case "response.failed":
			return "", cloudResponseError(0, event.Response.Error.Code)
		case "error":
			return "", cloudResponseError(0, event.Code)
		case "response.incomplete":
			return "", &CloudError{Kind: "incomplete"}
		case "response.completed":
			for _, item := range event.Response.Output {
				for _, content := range item.Content {
					if content.Type == "refusal" {
						return "", &CloudError{Kind: "refusal"}
					}
				}
			}
			if event.Response.Status != "completed" || output.Len() == 0 {
				return "", &CloudError{Kind: "incomplete"}
			}
			return output.String(), nil
		}
	}
	return "", &CloudError{Kind: "incomplete"}
}
