package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestManagedLLMAutoloadHonorsStartupAndOnDemandPolicies(t *testing.T) {
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	manager, err := llm.OpenManagedLlamaRuntimeManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		managedLLM:      manager,
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
	}
	t.Cleanup(func() {
		server.stopLLMAutoload()
		lifecycleCancel()
		manager.Close()
	})

	settings := config.DefaultSettings().LLM
	settings.ManagedLoadPolicy = config.LLMManagedLoadOnDemand
	server.startLLMAutoload(settings)
	if server.llmAutoloadID != 0 {
		t.Fatalf("on-demand policy scheduled preload id %d", server.llmAutoloadID)
	}

	settings.ManagedLoadPolicy = config.LLMManagedLoadStartup
	server.startLLMAutoload(settings)
	server.llmAutoloadWG.Wait()
	if server.llmAutoloadID == 0 {
		t.Fatal("startup policy did not schedule a preload attempt")
	}
}

func TestManagedLLMOnDemandTransitionReleasesCachedProvider(t *testing.T) {
	provider := &autoloadTestProvider{}
	server := &Server{llm: llmRuntime{cached: provider, cacheKey: "cached"}}
	previous := config.DefaultSettings().LLM
	next := previous
	next.ManagedLoadPolicy = config.LLMManagedLoadOnDemand

	if err := server.applyLLMSettingsTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	if provider.closed.Load() != 1 {
		t.Fatalf("provider close count = %d, want 1", provider.closed.Load())
	}
	if server.llm.cached != nil || server.llm.cacheKey != "" {
		t.Fatal("on-demand transition retained the cached managed provider")
	}
}

type autoloadTestProvider struct {
	closed atomic.Int32
}

func (*autoloadTestProvider) StreamChat(context.Context, llm.ChatRequest, func(string) error) (string, error) {
	return "", nil
}

func (*autoloadTestProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{}
}

func (p *autoloadTestProvider) Close() error {
	p.closed.Add(1)
	return nil
}

func TestManagedLLMWarmupUsesBoundedNonThinkingRequest(t *testing.T) {
	provider := &warmupTestProvider{err: llm.ErrOutputTruncated}
	if err := warmManagedLLM(t.Context(), provider, "managed-model"); err != nil {
		t.Fatalf("warmManagedLLM: %v", err)
	}
	request := provider.request
	if request.Model != "managed-model" || request.MaxTokens != 16 || request.Temperature != 0 ||
		request.ReasoningMode != "off" || len(request.Messages) != 2 {
		t.Fatalf("warmup request = %+v", request)
	}
}

func TestManagedLLMWarmupReportsProviderFailure(t *testing.T) {
	want := errors.New("warmup failed")
	provider := &warmupTestProvider{err: want}
	if err := warmManagedLLM(t.Context(), provider, "managed-model"); !errors.Is(err, want) {
		t.Fatalf("warmManagedLLM error = %v, want %v", err, want)
	}
}

type warmupTestProvider struct {
	request llm.ChatRequest
	err     error
}

func (p *warmupTestProvider) StreamChat(_ context.Context, request llm.ChatRequest, _ func(string) error) (string, error) {
	p.request = request
	return "", p.err
}

func (*warmupTestProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Available: true}
}
