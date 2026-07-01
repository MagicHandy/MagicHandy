// Package llm provides local model provider adapters.
package llm

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Message is one chat turn sent to a local model provider.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the provider-neutral streaming chat request.
type ChatRequest struct {
	Messages    []Message
	Model       string
	Temperature float64
}

// Provider streams text from one local LLM runtime.
type Provider interface {
	StreamChat(ctx context.Context, request ChatRequest, onDelta func(string) error) (string, error)
	Status(ctx context.Context) ProviderStatus
}

// ProviderStatus is a diagnostics-safe provider readiness snapshot.
type ProviderStatus struct {
	Provider  string `json:"provider"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	Available bool   `json:"available"`
	Message   string `json:"message,omitempty"`
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

func checkedRequestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}
