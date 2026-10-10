package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// CompatibleOptions declares protocol capabilities independently of llama.cpp.
// OpenRouter discovers model support; generic servers use the explicit saved
// capability policy, and a generic server on this computer with no saved policy
// gets the standard defaults described at localDefaults. Neither path emits
// local health or sampling extensions.
type CompatibleOptions struct {
	HTTPProviderOptions
	Provider            string
	Token               TokenSource
	OutputMode          string
	SupportedParameters []string
	AllowFallbacks      bool
	AllowedProviders    []string
	DataCollection      string
	ZeroDataRetention   bool
}

// HostedModel is provider catalog metadata, independent of local runtimes.
type HostedModel struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	SupportedParameters    []string `json:"supported_parameters,omitempty"`
	ReasoningEfforts       []string `json:"reasoning_efforts,omitempty"`
	DefaultReasoningEffort string   `json:"default_reasoning_effort,omitempty"`
}

// CompatibleProvider is an ordinary Chat Completions adapter, not a local
// llama.cpp server and not ChatGPT plan usage.
type CompatibleProvider struct {
	options        CompatibleOptions
	mu             sync.Mutex
	parameters     []string
	capabilitiesAt time.Time
}

// NewCompatibleProvider constructs a credential-bound Chat Completions client.
func NewCompatibleProvider(options CompatibleOptions) (*CompatibleProvider, error) {
	normalized, err := normalizeHTTPOptions(options.HTTPProviderOptions)
	if err != nil {
		return nil, err
	}
	options.HTTPProviderOptions = normalized
	options.Client = noRedirectClient(options.Client)
	if options.OutputMode == "" {
		options.OutputMode = "auto"
	}
	return &CompatibleProvider{options: options}, nil
}

// noRedirectClient prevents a credential following a redirect, even to another
// endpoint on the same host. Providers must serve their configured API directly.
func noRedirectClient(client *http.Client) *http.Client {
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &clientCopy
}

func (p *CompatibleProvider) request(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, p.options.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, &CloudError{Kind: "unavailable"}
	}
	if p.options.Token != nil {
		token, err := p.options.Token(ctx)
		if err != nil {
			return nil, err
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := p.options.Client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &CloudError{Kind: "unavailable"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = response.Body.Close()
		if method == http.MethodPost && response.StatusCode == http.StatusBadRequest && p.localDefaults() {
			// A local server that rejects the default structured request needs
			// another output mode on its connection, not a retry.
			return nil, &CloudError{Kind: "capability"}
		}
		return nil, cloudResponseError(response.StatusCode, "")
	}
	return response, nil
}

// localDefaults reports a generic server on this computer, such as LM Studio,
// llama-server, an MLX server, vLLM or Ollama, left on automatic output with no
// declared parameters. Such servers take the standard sampling parameters and
// the non-strict json_schema form, which constrains decoding to the domain
// schema; without it, local models write the contract unconstrained.
func (p *CompatibleProvider) localDefaults() bool {
	if p.options.Provider != "compatible" || p.options.OutputMode != "auto" || len(p.options.SupportedParameters) > 0 {
		return false
	}
	parsed, err := url.Parse(p.options.BaseURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// Models reads the provider's catalog without local health/load requests.
func (p *CompatibleProvider) Models(ctx context.Context) ([]HostedModel, error) {
	ctx, cancel := context.WithTimeout(ctx, p.options.Timeout)
	defer cancel()
	response, err := p.request(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	var catalog struct {
		Data []HostedModel `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&catalog) != nil {
		return nil, &CloudError{Kind: "unavailable"}
	}
	models := make([]HostedModel, 0, len(catalog.Data))
	for _, model := range catalog.Data {
		if ValidateCloudModel(model.ID) == nil {
			if model.Name == "" {
				model.Name = model.ID
			}
			models = append(models, model)
		}
	}
	return models, nil
}

// Status reports catalog availability; readiness still requires generation.
func (p *CompatibleProvider) Status(ctx context.Context) ProviderStatus {
	status := ProviderStatus{Provider: p.options.Provider, BaseURL: p.options.BaseURL, Model: p.options.Model}
	models, err := p.Models(ctx)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	status.Available = true
	for _, model := range models {
		status.Models = append(status.Models, model.ID)
		if model.ID == p.options.Model {
			status.ModelAvailable = true
		}
	}
	return status
}

func (p *CompatibleProvider) supportedParameters(ctx context.Context) ([]string, error) {
	if p.options.Provider != "openrouter" {
		return p.options.SupportedParameters, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.capabilitiesAt) < 15*time.Minute {
		return append([]string(nil), p.parameters...), nil
	}
	models, err := p.Models(ctx)
	if err != nil {
		return nil, err
	}
	index := slices.IndexFunc(models, func(model HostedModel) bool { return model.ID == p.options.Model })
	if index < 0 {
		return nil, &CloudError{Kind: "model"}
	}
	p.parameters = append([]string(nil), models[index].SupportedParameters...)
	p.capabilitiesAt = time.Now()
	return append([]string(nil), p.parameters...), nil
}

// StreamChat releases output only after a successful terminal stream event.
func (p *CompatibleProvider) StreamChat(ctx context.Context, request ChatRequest, onDelta func(string) error) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.options.Timeout)
	defer cancel()
	bounded, budget, err := BudgetChatRequest(request, p.options.ContextSize)
	if request.OnBudget != nil {
		request.OnBudget(budget)
	}
	if err != nil {
		return "", err
	}
	parameters, err := p.supportedParameters(ctx)
	if err != nil {
		return "", err
	}
	body, strict, err := p.completionBody(bounded, parameters)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	response, err := p.request(ctx, http.MethodPost, "/chat/completions", encoded)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := consumeCompatibleStream(response.Body, request.OnProgress)
	if err != nil {
		return "", err
	}
	if strict {
		raw, err = DomainOutput(raw, request.JSONSchema)
		if err != nil {
			return "", err
		}
	}
	if onDelta != nil {
		if err := onDelta(raw); err != nil {
			return "", err
		}
	}
	return raw, nil
}

func (p *CompatibleProvider) completionBody(request ChatRequest, parameters []string) (map[string]any, bool, error) {
	body := map[string]any{"model": p.options.Model, "messages": request.Messages, "stream": true}
	if p.localDefaults() {
		parameters = []string{"max_tokens", "temperature", "top_p"}
	}
	addCompatibleParameters(body, request, parameters)
	if p.options.Provider == "openrouter" {
		body["provider"] = map[string]any{"allow_fallbacks": p.options.AllowFallbacks, "require_parameters": true, "data_collection": p.options.DataCollection, "zdr": p.options.ZeroDataRetention}
		if len(p.options.AllowedProviders) > 0 {
			body["provider"].(map[string]any)["only"] = p.options.AllowedProviders
		}
		body["transforms"] = []string{}
	}
	return p.structuredCompletionBody(body, request.JSONSchema, parameters)
}

func addCompatibleParameters(body map[string]any, request ChatRequest, parameters []string) {
	for _, parameter := range parameters {
		switch parameter {
		case "max_tokens", "max_completion_tokens":
			if request.MaxTokens > 0 {
				body[parameter] = request.MaxTokens
			}
		case "temperature":
			body[parameter] = request.Temperature
		case "top_p":
			if request.TopP > 0 {
				body[parameter] = request.TopP
			}
		}
	}
}

func (p *CompatibleProvider) structuredCompletionBody(body map[string]any, schema json.RawMessage, parameters []string) (map[string]any, bool, error) {
	if len(schema) == 0 {
		return body, false, nil
	}
	mode := p.options.OutputMode
	if mode == "auto" {
		mode = "prompt"
		if slices.Contains(parameters, "structured_outputs") {
			mode = "strict"
		} else if slices.Contains(parameters, "response_format") {
			mode = "json"
		} else if p.localDefaults() {
			mode = "schema"
		}
	}
	if p.options.Provider == "openrouter" && mode == "strict" && !slices.Contains(parameters, "structured_outputs") {
		return nil, false, &CloudError{Kind: "capability"}
	}
	if p.options.Provider == "openrouter" && mode == "json" && !slices.Contains(parameters, "response_format") {
		return nil, false, &CloudError{Kind: "capability"}
	}
	switch mode {
	case "strict":
		schema, err := OpenAISchema(schema)
		if err != nil {
			return nil, false, err
		}
		body["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "magichandy_response", "strict": true, "schema": schema}}
		return body, true, nil
	case "schema":
		// Non-strict, with the original domain schema: the server constrains
		// decoding to it, and the semantic parser still validates the result.
		body["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "magichandy_response", "schema": schema}}
	case "json":
		body["response_format"] = map[string]string{"type": "json_object"}
	case "prompt":
	default:
		return nil, false, errors.New("unknown structured output policy")
	}
	return body, false, nil
}

type compatibleChunk struct {
	Error   json.RawMessage `json:"error"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Content string `json:"content"`
			Refusal string `json:"refusal"`
		} `json:"delta"`
	} `json:"choices"`
}

func consumeCompatibleStream(reader io.Reader, progress func(ProviderProgress)) (string, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 4*maxStreamResponseBytes))
	scanner.Buffer(make([]byte, 4096), maxStreamResponseBytes)
	var output strings.Builder
	finished := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			if finished && output.Len() > 0 {
				return output.String(), nil
			}
			return "", &CloudError{Kind: "incomplete"}
		}
		var chunk compatibleChunk
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			return "", &CloudError{Kind: "incomplete"}
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return "", &CloudError{Kind: "unavailable"}
		}
		if progress != nil {
			progress(ProviderProgress{Activity: true})
		}
		for _, choice := range chunk.Choices {
			if choice.Index != 0 {
				return "", &CloudError{Kind: "incomplete"}
			}
			if choice.Delta.Refusal != "" || choice.FinishReason == "content_filter" {
				return "", &CloudError{Kind: "refusal"}
			}
			if finished && choice.Delta.Content != "" {
				return "", &CloudError{Kind: "incomplete"}
			}
			if err := appendStreamDelta(&output, choice.Delta.Content, nil); err != nil {
				return "", &CloudError{Kind: "incomplete"}
			}
			switch choice.FinishReason {
			case "stop":
				finished = true
			case "":
			case "length":
				return "", &CloudError{Kind: "incomplete"}
			default:
				return "", &CloudError{Kind: "incomplete"}
			}
		}
	}
	return "", &CloudError{Kind: "incomplete"}
}
