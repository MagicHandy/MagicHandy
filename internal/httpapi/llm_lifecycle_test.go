package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestStatusResolutionCannotEvictActiveLabModel(t *testing.T) {
	s := newTestServer(t)
	s.stopLLMAutoload()
	active := &closeTrackingLLMProvider{}
	s.llm.cached, s.llm.cacheKey = active, "lab-model"
	settings := config.DefaultSettings().LLM
	settings.Provider = config.LLMProviderOllama
	if _, err := s.resolveLLMProvider(t.Context(), settings, false); err != nil {
		t.Fatal(err)
	}
	if active.closes.Load() != 0 || s.llm.cached != active {
		t.Fatal("status read evicted active model")
	}
}

func TestPreparedProviderCannotResolveAfterRuntimeRetirement(t *testing.T) {
	s := newTestServer(t)
	s.stopLLMAutoload()
	settings, _ := s.store.Snapshot()
	p, err := s.prepareLLMProvider(settings.LLM)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.closeLLM(); err != nil {
		t.Fatal(err)
	}
	// Generation rejection precedes any unavailable managed model setup work.
	_, err = p.StreamChat(t.Context(), llm.ChatRequest{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("retired request: %v", err)
	}
}

func TestRuntimeChangeKeepsAdmissionClosedUntilRetirementFinishes(t *testing.T) {
	var coordinator llmRequestCoordinator
	active, _, release, err := coordinator.acquire(t.Context(), llmRequestInteractive)
	if err != nil {
		t.Fatal(err)
	}
	finishChange := coordinator.beginChange()
	if active.Err() == nil {
		t.Fatal("runtime change did not cancel active generation")
	}
	release()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, _, _, err := coordinator.acquire(ctx, llmRequestInteractive); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("generation entered during retirement: %v", err)
	}
	finishChange()
	_, _, release, err = coordinator.acquire(t.Context(), llmRequestInteractive)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
