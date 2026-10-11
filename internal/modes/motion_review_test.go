package modes

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestSupersededReviewReleasesOnlyItsOwnHandoff(t *testing.T) {
	for _, outcome := range []string{"canceled", "committed", "new_handoff"} {
		t.Run(outcome, func(t *testing.T) {
			engine := &fakeEngine{}
			clock := &fakeClock{now: time.Unix(0, 0)}
			pattern := &motion.PatternDefinition{ID: "custom-wave", Name: "Custom wave"}
			decider := &fakeDecider{decisions: []Decision{{Segment: Segment{PatternID: pattern.ID, SpeedPercent: 30, DurationMillis: 4000}, Pattern: pattern}}}
			manager := newAutopilotManager(t, engine, clock, decider, nil)
			if _, err := manager.Start(t.Context(), ModeAutopilot); err != nil {
				t.Fatal(err)
			}
			waitForAutonomousStart(t, manager, engine)
			ticket, eligible := manager.MotionReviewTicket()
			if !eligible {
				t.Fatal("running Autopilot is not reviewable")
			}
			target := engine.Snapshot().Target
			entered, release := make(chan struct{}), make(chan struct{})
			finished := make(chan error, 1)
			go func() {
				finished <- manager.ApplyMotionReview(t.Context(), ticket, target, func(ctx context.Context) (motion.ActiveMotionState, error) {
					close(entered)
					<-release // Model a handoff that takes time to acknowledge cancellation.
					if outcome == "committed" {
						return engine.Snapshot(), nil
					}
					return motion.ActiveMotionState{}, ctx.Err()
				})
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				close(release)
				t.Fatal("review did not enter the handoff")
			}
			activity := manager.NotifyChatActivity()
			var newer uint64
			if outcome == "new_handoff" {
				newer = manager.PrepareChatTarget()
			}
			close(release)
			if err := <-finished; err == nil {
				t.Fatal("superseded correction reported success")
			}
			manager.mu.Lock()
			pending := manager.chat.pending
			manager.mu.Unlock()
			if pending != (outcome == "new_handoff") {
				t.Fatalf("pending=%v after %s", pending, outcome)
			}
			if outcome == "new_handoff" {
				manager.CancelChatTarget(newer)
			}
			// Complete a chat turn with no motion edit. Its normal reconsideration
			// must be able to schedule another model decision.
			before := decider.callCount()
			manager.ReconsiderAfterChat()
			manager.NotifyChatActivityComplete(activity)
			waitFor(t, time.Second, func() bool { return decider.callCount() > before })
		})
	}
}
