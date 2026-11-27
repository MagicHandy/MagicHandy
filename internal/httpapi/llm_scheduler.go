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
	activeCancel       context.CancelCauseFunc
	interactiveWaiters int
	changed            chan struct{}
	generation         uint64
	changing           int
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
	c.mu.Lock()
	generation := c.generation
	c.mu.Unlock()

	for {
		c.mu.Lock()
		// Cancellation wins over an available slot or a simultaneous release.
		// A dead interactive request must not preempt valid autonomous work.
		err := ctx.Err()
		if generation != c.generation {
			err = errLLMSettingsChanged
		}
		if err != nil {
			if registered {
				c.interactiveWaiters--
				c.signalLocked()
			}
			c.mu.Unlock()
			return nil, time.Since(started), nil, err
		}
		if c.changed == nil {
			c.changed = make(chan struct{})
		}
		if interactive && !registered {
			c.interactiveWaiters++
			registered = true
			if c.active && c.activePriority == llmRequestAutonomous && c.activeCancel != nil {
				c.activeCancel(context.Canceled)
			}
			c.signalLocked()
		}
		if !c.active && c.changing == 0 && (interactive || c.interactiveWaiters == 0) {
			leaseCtx, leaseCancel := context.WithCancelCause(ctx)
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
					leaseCancel(nil)
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
		case <-changed:
		}
	}
}

// invalidate cancels admitted work and rejects already waiting requests when
// the model configuration changes. New arrivals can use the replacement.
func (c *llmRequestCoordinator) invalidate() {
	c.mu.Lock()
	c.generation++
	if c.activeCancel != nil {
		c.activeCancel(errLLMSettingsChanged)
	}
	c.signalLocked()
	c.mu.Unlock()
}

// beginChange prevents newly arriving work from loading a replacement model
// while the settings transition is still retiring the old runtime.
func (c *llmRequestCoordinator) beginChange() func() {
	c.mu.Lock()
	c.changing++
	c.generation++
	if c.activeCancel != nil {
		c.activeCancel(errLLMSettingsChanged)
	}
	c.signalLocked()
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			c.changing--
			c.signalLocked()
			c.mu.Unlock()
		})
	}
}

func (c *llmRequestCoordinator) signalLocked() {
	if c.changed != nil {
		close(c.changed)
	}
	c.changed = make(chan struct{})
}

type llmCallTiming struct {
	historyLookup bool
	started       time.Time
	firstToken    time.Time
	finished      time.Time
	budget        llm.PromptBudget
	firstActivity time.Time
	progress      llm.ProviderProgress
}

// timedLLMProvider records provider phases without changing the request,
// callback, output, or retry/repair behavior owned by the chat services.
type timedLLMProvider struct {
	provider       llm.Provider
	mu             sync.Mutex
	calls          []llmCallTiming
	initialRequest *llm.ChatRequest
}

func newTimedLLMProvider(provider llm.Provider) *timedLLMProvider {
	return &timedLLMProvider{provider: provider}
}

func (p *timedLLMProvider) StreamChat(
	ctx context.Context,
	request llm.ChatRequest,
	onDelta func(string) error,
) (string, error) {
	call := llmCallTiming{started: time.Now(), historyLookup: request.HistoryLookup}
	previousProgress := request.OnProgress
	request.OnProgress = func(progress llm.ProviderProgress) {
		if progress.Activity && call.firstActivity.IsZero() {
			call.firstActivity = time.Now()
		}
		call.progress.LoadMillis = max(call.progress.LoadMillis, progress.LoadMillis)
		call.progress.PromptEvalMillis = max(call.progress.PromptEvalMillis, progress.PromptEvalMillis)
		call.progress.PromptTokens = max(call.progress.PromptTokens, progress.PromptTokens)
		call.progress.CachedPromptTokens = max(call.progress.CachedPromptTokens, progress.CachedPromptTokens)
		call.progress.GeneratedTokens = max(call.progress.GeneratedTokens, progress.GeneratedTokens)
		if previousProgress != nil {
			previousProgress(progress)
		}
	}
	previousBudget := request.OnBudget
	request.OnBudget = func(budget llm.PromptBudget) {
		call.budget = budget
		if previousBudget != nil {
			previousBudget(budget)
		}
	}
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
	if p.initialRequest == nil || (p.initialRequest.HistoryLookup && !request.HistoryLookup) {
		request.OnProgress, request.OnBudget = nil, nil
		p.initialRequest = &request
	}
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
	if !calls[0].firstActivity.IsZero() {
		diagnostics.FirstActivityMillis = calls[0].firstActivity.Sub(requestStarted).Milliseconds()
	}
	ordinaryCallSeen := false
	for _, call := range calls {
		millis := call.finished.Sub(call.started).Milliseconds()
		if !call.historyLookup && ordinaryCallSeen {
			diagnostics.RepairMillis += millis
			continue
		}
		ordinaryCallSeen = ordinaryCallSeen || !call.historyLookup
		diagnostics.GenerationMillis += millis
		diagnostics.ModelLoadMillis += call.progress.LoadMillis
		diagnostics.PromptEvalMillis += call.progress.PromptEvalMillis
		diagnostics.PromptTokens += call.progress.PromptTokens
		diagnostics.CachedPromptTokens += call.progress.CachedPromptTokens
		diagnostics.GeneratedTokens += call.progress.GeneratedTokens
		diagnostics.HistoryMessagesDropped = max(diagnostics.HistoryMessagesDropped, call.budget.HistoryMessagesDropped)
		if call.budget.InputBytes >= diagnostics.PromptBytes {
			diagnostics.PromptBytes = call.budget.InputBytes
			diagnostics.PromptLimitBytes = call.budget.LimitBytes
		}
	}
}
