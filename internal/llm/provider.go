// Package llm provides local model provider adapters.
package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrOutputTruncated reports that a provider exhausted its generation budget.
var ErrOutputTruncated = errors.New("LLM output reached the maximum token limit")

const maxStreamResponseBytes = 1 << 20

var (
	errIncompleteStream = errors.New("LLM stream ended before a completion marker")
	errResponseTooLarge = errors.New("LLM response exceeded the stream size limit")
)

// Message is one chat turn sent to a local model provider.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the provider-neutral streaming chat request.
type ChatRequest struct {
	Messages      []Message
	Model         string
	Temperature   float64
	MaxTokens     int
	ReasoningMode string
	// ReasoningBudgetTokens is used only by providers with an explicit bounded-thinking API.
	ReasoningBudgetTokens int
}

// Provider streams text from one local LLM runtime.
type Provider interface {
	StreamChat(ctx context.Context, request ChatRequest, onDelta func(string) error) (string, error)
	Status(ctx context.Context) ProviderStatus
}

// LoadableProvider can explicitly load and unload provider runtime state.
type LoadableProvider interface {
	Provider

	Load(ctx context.Context) ProviderStatus
	Unload(ctx context.Context) ProviderStatus
}

// ProviderStatus is a diagnostics-safe provider readiness snapshot.
type ProviderStatus struct {
	Provider       string   `json:"provider"`
	BaseURL        string   `json:"base_url"`
	Model          string   `json:"model"`
	Available      bool     `json:"available"`
	ModelAvailable bool     `json:"model_available"`
	Managed        bool     `json:"managed"`
	Loaded         bool     `json:"loaded"`
	Models         []string `json:"models,omitempty"`
	Message        string   `json:"message,omitempty"`
}

// HTTPProviderOptions configures a local HTTP-backed provider.
type HTTPProviderOptions struct {
	BaseURL string
	Model   string
	Client  *http.Client
	Timeout time.Duration
}

func normalizeHTTPOptions(options HTTPProviderOptions) (HTTPProviderOptions, error) {
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	options.Model = strings.TrimSpace(options.Model)
	if options.BaseURL == "" {
		return HTTPProviderOptions{}, errors.New("LLM provider base URL is required")
	}
	parsed, err := url.Parse(options.BaseURL)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return HTTPProviderOptions{}, fmt.Errorf("LLM provider base URL must be an absolute HTTP URL with a host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return HTTPProviderOptions{}, errors.New("LLM provider base URL scheme must be http or https")
	}
	if parsed.User != nil {
		return HTTPProviderOptions{}, errors.New("LLM provider base URL must not include userinfo")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return HTTPProviderOptions{}, errors.New("LLM provider base URL must not include a query")
	}
	if parsed.Fragment != "" {
		return HTTPProviderOptions{}, errors.New("LLM provider base URL must not include a fragment")
	}
	if options.Model == "" {
		return HTTPProviderOptions{}, errors.New("LLM model is required")
	}
	if options.Timeout <= 0 {
		options.Timeout = 120 * time.Second
	}
	if options.Client == nil {
		options.Client = &http.Client{Timeout: options.Timeout}
	}
	return options, nil
}

func appendStreamDelta(builder *strings.Builder, delta string, onDelta func(string) error) error {
	if len(delta) > maxStreamResponseBytes-builder.Len() {
		return errResponseTooLarge
	}
	builder.WriteString(delta)
	if onDelta != nil {
		return onDelta(delta)
	}
	return nil
}

func checkedRequestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}
