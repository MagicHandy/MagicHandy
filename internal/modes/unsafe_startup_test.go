package modes

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestUnsafeStartupCannotRetryWhileTeardownWaits(t *testing.T) {
	engine := &fakeEngine{startErr: fmt.Errorf("%w: outside recoverable travel", motion.ErrUnsafeStartupState)}
	clock := &fakeClock{now: time.Unix(0, 0)}
	decider := &fakeDecider{decisions: []Decision{{Segment: Segment{
		Dynamic: &motion.DynamicDefinition{CenterPercent: 50, SpanPercent: 30}, SpeedPercent: 28,
	}}}}
	manager := newAutopilotManager(t, engine, clock, decider, nil)
	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Drive ticks explicitly while lifecycle teardown is held by another
	// operation. This makes the reported CI scheduling race deterministic.
	manager.loop.mode = ModeAutopilot
	manager.loop.generation = 1
	manager.loop.cancel = cancel
	manager.lifecycleMu.Lock()
	releaseTeardown := sync.OnceFunc(manager.lifecycleMu.Unlock)
	defer releaseTeardown()
	manager.tickAutopilot(loopCtx)
	clock.Advance(time.Hour)
	manager.tickAutopilot(loopCtx)
	if calls := decider.callCount(); calls != 1 {
		t.Fatalf("unsafe startup decisions while teardown waits = %d, want 1", calls)
	}
	if loopCtx.Err() == nil {
		t.Fatal("unsafe startup did not immediately cancel its loop")
	}
	if _, finish, admitted := manager.beginStartOperation(loopCtx, ModeAutopilot, 1, 0); admitted {
		finish()
		t.Fatal("a canceled loop admitted work before teardown completed")
	}
	releaseTeardown()
	waitFor(t, time.Second, func() bool { return !manager.Status().Active })
}
