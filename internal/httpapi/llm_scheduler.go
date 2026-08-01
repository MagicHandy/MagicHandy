package httpapi

import (
	"context"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type llmRequestPriority uint8

const (
	llmRequestAutonomous llmRequestPriority = iota
	llmRequestInteractive
)

// llmRequestCoordinator keeps one local-model stream active at a time and
// prevents a newly scheduled autonomous turn from overtaking waiting chat.
// The zero value is ready for use so focused Server tests do not need setup.
type llmRequestCoordinator struct {
	mu                 sync.Mutex
	active             bool
	activePriority     llmRequestPriority
	activeCancel       context.CancelFunc
	interactiveWaiters int
	changed            chan struct{}
}

func (c *llmRequestCoordinator) acquire(
	ctx context.Context,
	priority llmRequestPriority,
) (context.Context, time.Duration, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	interactive := priority == llmRequestInteractive
	registered := false

	for {
		c.mu.Lock()
		if c.changed == nil {
			c.changed = make(chan struct{})
		}
		if interactive && !registered {
			c.interactiveWaiters++
			registered = true
			if c.active && c.activePriority == llmRequestAutonomous && c.activeCancel != nil {
				c.activeCancel()
			}
			c.signalLocked()
		}
		if !c.active && (interactive || c.interactiveWaiters == 0) {
			leaseCtx, leaseCancel := context.WithCancel(ctx)
			c.active = true
			c.activePriority = priority
			c.activeCancel = leaseCancel
			if registered {
				c.interactiveWaiters--
			}
			c.signalLocked()
			c.mu.Unlock()

			var once sync.Once
			return leaseCtx, time.Since(started), func() {
				once.Do(func() {
					leaseCancel()
					c.mu.Lock()
					c.active = false
					c.activeCancel = nil
					c.signalLocked()
					c.mu.Unlock()
				})
			}, nil
		}
		changed := c.changed
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			c.mu.Lock()
			if registered {
				c.interactiveWaiters--
				c.signalLocked()
			}
			c.mu.Unlock()
			return nil, time.Since(started), nil, ctx.Err()
		case <-changed:
		}
	}
}

func (c *llmRequestCoordinator) signalLocked() {
	if c.changed != nil {
		close(c.changed)
	}
	c.changed = make(chan struct{})
}

type llmCallTiming struct {
	started    time.Time
	firstToken time.Time
	finished   time.Time
}

// timedLLMProvider records provider phases without changing the request,
// callback, output, or retry/repair behavior owned by the chat services.
type timedLLMProvider struct {
	provider llm.Provider
	mu       sync.Mutex
	calls    []llmCallTiming
}

func newTimedLLMProvider(provider llm.Provider) *timedLLMProvider {
	return &timedLLMProvider{provider: provider}
}

func (p *timedLLMProvider) StreamChat(
	ctx context.Context,
	request llm.ChatRequest,
	onDelta func(string) error,
) (string, error) {
	call := llmCallTiming{started: time.Now()}
	var firstOnce sync.Once
	wrappedDelta := func(delta string) error {
		firstOnce.Do(func() { call.firstToken = time.Now() })
		if onDelta == nil {
			return nil
		}
		return onDelta(delta)
	}
	raw, err := p.provider.StreamChat(ctx, request, wrappedDelta)
	call.finished = time.Now()
	if call.firstToken.IsZero() && raw != "" {
		call.firstToken = call.finished
	}
	p.mu.Lock()
	p.calls = append(p.calls, call)
	p.mu.Unlock()
	return raw, err
}

func (p *timedLLMProvider) Status(ctx context.Context) llm.ProviderStatus {
	return p.provider.Status(ctx)
}

func (p *timedLLMProvider) applyDiagnostics(
	diagnostics *chat.MessageDiagnostics,
	requestStarted time.Time,
	preparation time.Duration,
	schedulerWait time.Duration,
) {
	if diagnostics == nil {
		return
	}
	p.mu.Lock()
	calls := append([]llmCallTiming(nil), p.calls...)
	p.mu.Unlock()

	diagnostics.SchedulerWaitMillis = schedulerWait.Milliseconds()
	diagnostics.ProviderCalls = len(calls)
	if len(calls) == 0 {
		diagnostics.PreparationMillis = preparation.Milliseconds()
		return
	}
	preparationMillis := calls[0].started.Sub(requestStarted).Milliseconds() - diagnostics.SchedulerWaitMillis
	diagnostics.PreparationMillis = max(0, preparationMillis)
	if !calls[0].firstToken.IsZero() {
		diagnostics.FirstTokenMillis = calls[0].firstToken.Sub(requestStarted).Milliseconds()
	}
	diagnostics.GenerationMillis = calls[0].finished.Sub(calls[0].started).Milliseconds()
	for _, call := range calls[1:] {
		diagnostics.RepairMillis += call.finished.Sub(call.started).Milliseconds()
	}
}
