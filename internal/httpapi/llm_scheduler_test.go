package httpapi

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLLMRequestCoordinatorPrioritizesWaitingChat(t *testing.T) {
	var coordinator llmRequestCoordinator
	_, _, releaseActive, err := coordinator.acquire(context.Background(), llmRequestAutonomous)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan string, 2)
	releaseAutonomous := make(chan struct{})
	go func() {
		_, _, release, acquireErr := coordinator.acquire(context.Background(), llmRequestAutonomous)
		if acquireErr != nil {
			return
		}
		acquired <- "autonomous"
		<-releaseAutonomous
		release()
	}()
	go func() {
		_, _, release, acquireErr := coordinator.acquire(context.Background(), llmRequestInteractive)
		if acquireErr != nil {
			return
		}
		acquired <- "interactive"
		release()
	}()

	waitForInteractiveLLMWaiter(t, &coordinator)
	releaseActive()
	select {
	case got := <-acquired:
		if got != "interactive" {
			t.Fatalf("first request after release = %q, want interactive", got)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting interactive request did not acquire the model")
	}
	close(releaseAutonomous)
}

func TestLLMRequestCoordinatorRemovesCanceledInteractiveWaiter(t *testing.T) {
	var coordinator llmRequestCoordinator
	_, _, release, err := coordinator.acquire(context.Background(), llmRequestAutonomous)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, _, acquireErr := coordinator.acquire(ctx, llmRequestInteractive)
		done <- acquireErr
	}()
	waitForInteractiveLLMWaiter(t, &coordinator)
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("canceled waiter error = %v", err)
	}
	coordinator.mu.Lock()
	waiters := coordinator.interactiveWaiters
	coordinator.mu.Unlock()
	if waiters != 0 {
		t.Fatalf("interactive waiters after cancellation = %d", waiters)
	}
	release()
}

func TestLLMRequestCoordinatorPreemptsActiveAutonomousInference(t *testing.T) {
	var coordinator llmRequestCoordinator
	autonomousCtx, _, releaseAutonomous, err := coordinator.acquire(context.Background(), llmRequestAutonomous)
	if err != nil {
		t.Fatal(err)
	}
	interactiveAcquired := make(chan struct{})
	go func() {
		_, _, release, acquireErr := coordinator.acquire(context.Background(), llmRequestInteractive)
		if acquireErr != nil {
			return
		}
		close(interactiveAcquired)
		release()
	}()

	select {
	case <-autonomousCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("waiting interactive request did not cancel active autonomous inference")
	}
	select {
	case <-interactiveAcquired:
		t.Fatal("interactive request acquired before the autonomous lease was released")
	default:
	}
	releaseAutonomous()
	select {
	case <-interactiveAcquired:
	case <-time.After(time.Second):
		t.Fatal("interactive request did not acquire after autonomous release")
	}
}

func waitForInteractiveLLMWaiter(t *testing.T, coordinator *llmRequestCoordinator) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		coordinator.mu.Lock()
		waiters := coordinator.interactiveWaiters
		coordinator.mu.Unlock()
		if waiters > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("interactive request did not enter the wait queue")
}

type timingTestProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *timingTestProvider) StreamChat(_ context.Context, _ llm.ChatRequest, onDelta func(string) error) (string, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	time.Sleep(8 * time.Millisecond)
	if err := onDelta("{"); err != nil {
		return "", err
	}
	time.Sleep(8 * time.Millisecond)
	return "{}", nil
}

func (p *timingTestProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Available: true}
}

func TestTimedLLMProviderAttributesInitialAndRepairCalls(t *testing.T) {
	requestStarted := time.Now()
	time.Sleep(3 * time.Millisecond)
	provider := newTimedLLMProvider(&timingTestProvider{})
	if _, err := provider.StreamChat(context.Background(), llm.ChatRequest{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.StreamChat(context.Background(), llm.ChatRequest{}, nil); err != nil {
		t.Fatal(err)
	}
	diagnostics := chat.MessageDiagnostics{}
	provider.applyDiagnostics(&diagnostics, requestStarted, 3*time.Millisecond, 0)
	if diagnostics.ProviderCalls != 2 || diagnostics.FirstTokenMillis < 8 ||
		diagnostics.GenerationMillis < 12 || diagnostics.RepairMillis < 12 {
		t.Fatalf("timing diagnostics = %+v", diagnostics)
	}
}
