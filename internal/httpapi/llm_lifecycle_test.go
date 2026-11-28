package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

func TestLocalRuntimeEditDrainsOnlyLocalWorkDuringHostedChat(t *testing.T) {
	s, _, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) { writeHostedFixtureReply(w, `{"reply":"Ready"}`) })
	s.stopLLMAutoload()
	before, _ := s.store.Snapshot()
	prepared, err := s.prepareLLMProvider(before.LLM)
	if err != nil {
		t.Fatal(err)
	}
	hosted, _, doneHosted, err := s.acquireProviderRequest(t.Context(), llmRequestInteractive, before.LLM)
	if err != nil {
		t.Fatal(err)
	}
	defer doneHosted()
	local, _, doneLocal, err := s.llmRequests.acquire(t.Context(), llmRequestInteractive)
	if err != nil {
		t.Fatal(err)
	}
	defer doneLocal()
	worker := &closeTrackingLLMProvider{}
	s.llm.cached, s.llm.cacheKey = worker, "old"
	saveSettings(t, s.store, func(v config.Settings) config.Settings { v.LLM.Model = "new-local-model"; return v })
	after, _ := s.store.Snapshot()
	transition := make(chan error, 1)
	go func() { transition <- s.applySettingsRuntimeTransition(t.Context(), before, after) }()
	select {
	case <-local.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("local lease was not canceled")
	}
	if worker.closes.Load() != 0 {
		t.Error("closed the worker before its generation drained")
	}
	doneLocal()
	if err := <-transition; err != nil {
		t.Fatal(err)
	}
	if hosted.Err() != nil || worker.closes.Load() != 1 {
		t.Fatalf("hosted = %v, local closes = %d", hosted.Err(), worker.closes.Load())
	}
	raw, err := prepared.StreamChat(hosted, llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "Ready?"}}}, nil)
	if err != nil || raw == "" {
		t.Fatalf("existing hosted turn after local edit: %q, %v", raw, err)
	}
}

type unloadTrackingProvider struct {
	closeTrackingLLMProvider
	unloads atomic.Int32
}

func (*unloadTrackingProvider) Load(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Managed: true, Loaded: true}
}
func (p *unloadTrackingProvider) Unload(context.Context) llm.ProviderStatus {
	p.unloads.Add(1)
	return llm.ProviderStatus{Provider: config.LLMProviderLlamaCPP, Model: "local", Managed: true, Message: "unloaded"}
}

func TestSplitRoleControlsKeepHostedStatusAndUnloadLocalWorker(t *testing.T) {
	var contacts atomic.Int32
	s, _, connection := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		contacts.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"id":"fixture-model"}]}`))
	})
	s.stopLLMAutoload()
	worker := &unloadTrackingProvider{}
	s.llm.cached = worker
	call := func(method, path string) llm.ProviderStatus {
		t.Helper()
		r := httptest.NewRecorder()
		s.Handler().ServeHTTP(r, withController(httptest.NewRequest(method, path, nil)))
		if r.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body.String())
		}
		var status llm.ProviderStatus
		decodeResponse(t, r, &status)
		return status
	}
	if status := call(http.MethodGet, "/api/llm/status"); status.Model != connection.Model {
		t.Fatal(status)
	}
	count := contacts.Load()
	if status := call(http.MethodGet, "/api/llm/status?role=local"); !status.Managed || status.Model == connection.Model {
		t.Fatal(status)
	}
	if status := call(http.MethodPost, "/api/llm/unload?role=local"); status.Model != "local" || worker.unloads.Load() != 1 || contacts.Load() != count {
		t.Fatal(status)
	}
	local, _, release, err := s.llmRequests.acquire(t.Context(), llmRequestInteractive)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if status := call(http.MethodPost, "/api/llm/unload"); status.Model != connection.Model || local.Err() != nil || worker.unloads.Load() != 1 {
		t.Fatal(status, local.Err())
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
