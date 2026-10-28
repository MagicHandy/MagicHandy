package httpapi

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// Chat can arrive between an Autopilot retarget and its Cloud acknowledgement.
// Canceling the superseded decision must not turn that accepted edit into a
// transport failure, Stop, and full startup when the interactive reply arrives.
func TestChatDuringAutopilotUploadKeepsTheSameMotionStream(t *testing.T) {
	owner := &chatOverlapTransport{Fake: transport.NewFake(), entered: make(chan struct{}), release: make(chan struct{})}
	engine, err := motion.NewEngine(motion.EngineOptions{
		Transport: owner, DispatchInterval: time.Hour, ChunkSize: 8, SampleInterval: 125 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	wrapped := &chatOverlapEngine{Engine: engine, owner: owner, applied: make(chan struct{})}
	settings := config.DefaultSettings().Motion
	manager, err := modes.NewManager(modes.Options{
		Ensure:   func(context.Context) (modes.Engine, error) { return wrapped, nil },
		Current:  func() modes.Engine { return wrapped },
		Settings: func() config.MotionSettings { return settings },
		Tick:     5 * time.Millisecond, MaxSegmentDuration: 30 * time.Millisecond,
		Decide: func(context.Context, modes.DecisionInput) (modes.Decision, error) {
			score := chat.DefaultLayeredScore(35)
			return modes.Decision{Segment: modes.Segment{Flow: &score, SpeedPercent: 35, DurationMillis: 1000}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start(t.Context(), modes.ModeAutopilot); err != nil {
		t.Fatal(err)
	}
	select {
	case <-owner.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("Autopilot did not reach the blocked retarget upload")
	}
	before := engine.Snapshot()
	activity := manager.NotifyChatActivity()
	defer manager.NotifyChatActivityComplete(activity)
	close(owner.release)
	select {
	case <-wrapped.applied:
	case <-time.After(time.Second):
		t.Fatal("superseded Autopilot upload did not finish")
	}
	if state := engine.Snapshot(); !state.Running || state.StreamID != before.StreamID {
		t.Fatalf("chat interrupted the running stream: running=%v stream=%q error=%q", state.Running, state.StreamID, state.LastError)
	}
	// The interactive result must be an ordinary update of that same stream.
	generation := manager.PrepareChatTarget()
	target := before.Target
	target.Label, target.Source = "interactive pace variation", "chat"
	target.SpeedPercent = 42
	target.Flow = motion.CloneFlowSpec(target.Flow)
	target.Flow.SpeedPercent = 42
	if _, err := engine.ApplyTarget(t.Context(), target, "chat_target"); err != nil {
		t.Fatal(err)
	}
	if !manager.NotifyChatTarget(generation, target) {
		t.Fatal("interactive target was not adopted")
	}
	plays := 0
	for _, command := range owner.Commands() {
		if command.Kind == transport.CommandKindStop {
			t.Fatal("chat caused a transport Stop")
		}
		if command.Kind == transport.CommandKindPointsPlay {
			plays++
		}
	}
	if state := engine.Snapshot(); plays != 1 || !state.Running || state.StreamID != before.StreamID || state.Target.SpeedPercent != 42 {
		t.Fatalf("interactive update restarted or lost the stream: plays=%d state=%+v", plays, state)
	}
}

type chatOverlapEngine struct {
	*motion.Engine
	owner   *chatOverlapTransport
	applied chan struct{}
	once    sync.Once
}

func (e *chatOverlapEngine) ApplyTarget(ctx context.Context, target motion.MotionTarget, reason string) (motion.ActiveMotionState, error) {
	first := false
	e.once.Do(func() {
		first = true
		e.owner.mu.Lock()
		e.owner.block = true
		e.owner.mu.Unlock()
	})
	state, err := e.Engine.ApplyTarget(ctx, target, reason)
	if first {
		close(e.applied)
	}
	return state, err
}

type chatOverlapTransport struct {
	*transport.Fake
	mu               sync.Mutex
	block            bool
	entered, release chan struct{}
}

func (o *chatOverlapTransport) AppendPoints(ctx context.Context, command transport.AppendPointsCommand) (transport.CommandResult, error) {
	o.mu.Lock()
	block := o.block
	o.block = false
	o.mu.Unlock()
	result, err := o.Fake.AppendPoints(ctx, command)
	if !block || err != nil {
		return result, err
	}
	close(o.entered)
	select {
	case <-o.release:
	case <-ctx.Done():
	}
	// Even if release and cancellation race, an aborted HTTP request cannot
	// provide an acknowledgement for the accepted payload.
	return result, ctx.Err()
}
