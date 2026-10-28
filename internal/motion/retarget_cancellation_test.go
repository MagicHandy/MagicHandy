package motion

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestCanceledQueuedRetargetDoesNotChangeOrStopMotion(t *testing.T) {
	owner := transport.NewFake()
	engine := newTestEngine(t, owner, nil, time.Hour)
	before, err := engine.Start(t.Context(), testTarget(), config.DefaultSettings().Motion)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	commands := len(owner.Commands())
	engine.targetMu.Lock()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := engine.ApplyTarget(ctx, testTarget(), "stale"); done <- err }()
	cancel()
	engine.targetMu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if state := engine.Snapshot(); !state.Running || state.PlanID != before.PlanID || len(owner.Commands()) != commands {
		t.Fatal("canceled queued edit changed or stopped motion")
	}
}

func TestRetargetCancellationAtUploadBoundaries(t *testing.T) {
	for _, stage := range []string{"lead_refill", "committed", "pause", "transport_failure"} {
		t.Run(stage, func(t *testing.T) {
			owner := &retargetAckTransport{Fake: transport.NewFake(), entered: make(chan context.Context, 1), release: make(chan struct{})}
			engine := newTestEngine(t, owner, nil, time.Hour)
			before, err := engine.Start(t.Context(), testTarget(), config.DefaultSettings().Motion)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
			if stage != "lead_refill" {
				if err := engine.ensureLeadBuffer(t.Context(), engine.runEpoch, "prime"); err != nil {
					t.Fatal(err)
				}
			}
			if stage == "transport_failure" {
				owner.err = context.DeadlineExceeded
			}
			owner.block.Store(true)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			target := testTarget()
			target.SpeedPercent = 42
			go func() { _, err := engine.ApplyTarget(ctx, target, "retarget"); done <- err }()
			var wireCtx context.Context
			select {
			case wireCtx = <-owner.entered:
			case <-time.After(time.Second):
				t.Fatal("upload did not reach transport")
			}
			deadline, bounded := wireCtx.Deadline()
			if !bounded || time.Until(deadline) > targetDispatchTimeout {
				t.Fatal("committed upload has no bounded deadline")
			}
			committed := engine.Snapshot().PlanID != before.PlanID
			if committed != (stage != "lead_refill") {
				t.Fatalf("wrong cancellation boundary: committed=%v", committed)
			}
			cancel()
			if stage == "pause" {
				if _, err := engine.Pause(t.Context(), "pause_during_upload"); err != nil {
					t.Fatal(err)
				}
			} else {
				close(owner.release)
			}
			select {
			case err = <-done:
			case <-time.After(time.Second):
				t.Fatal("upload did not finish")
			}
			assertRetargetCancellationState(t, stage, before, engine.Snapshot(), err)
			assertRetargetCancellationCommands(t, stage, owner.Commands())
		})
	}
}

func assertRetargetCancellationState(t *testing.T, stage string, before, state ActiveMotionState, err error) {
	t.Helper()
	switch stage {
	case "lead_refill":
		if !errors.Is(err, context.Canceled) || !state.Running || state.PlanID != before.PlanID {
			t.Fatalf("canceled refill changed/stopped the plan: state=%+v err=%v", state, err)
		}
	case "committed":
		if err != nil || !state.Running || state.StreamID != before.StreamID || state.Target.SpeedPercent != 42 {
			t.Fatalf("committed update did not finish on the same stream: state=%+v err=%v", state, err)
		}
	case "pause":
		if err == nil || state.Running || !state.Paused {
			t.Fatalf("Pause failed to cancel committed upload: state=%+v err=%v", state, err)
		}
	case "transport_failure":
		if err == nil || state.Running {
			t.Fatalf("transport failure did not stop playback: state=%+v err=%v", state, err)
		}
	}
}

func assertRetargetCancellationCommands(t *testing.T, stage string, commands []transport.Command) {
	t.Helper()
	stops := countCommands(commands, transport.CommandKindStop)
	if stage == "pause" || stage == "transport_failure" {
		if stops != 1 || commands[len(commands)-1].Kind != transport.CommandKindStop {
			t.Fatal("Stop was not the final wire command")
		}
	} else if stops != 0 || countCommands(commands, transport.CommandKindPointsPlay) != 1 {
		t.Fatal("request cancellation stopped or restarted the stream")
	}
}

type retargetAckTransport struct {
	*transport.Fake
	block   atomic.Bool
	entered chan context.Context
	release chan struct{}
	err     error
}

func (o *retargetAckTransport) AppendPoints(ctx context.Context, command transport.AppendPointsCommand) (transport.CommandResult, error) {
	result, err := o.Fake.AppendPoints(ctx, command)
	if err != nil || !o.block.Swap(false) {
		return result, err
	}
	o.entered <- ctx
	select {
	case <-o.release:
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, o.err
	case <-ctx.Done():
		return result, ctx.Err()
	}
}
