package modes

import (
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
)

func TestOlderChatCompletionCannotResumePlanningDuringNewTurn(t *testing.T) {
	manager := newTestManager(t, &fakeEngine{}, &fakeClock{now: time.Now()}, diagnostics.NewTraceRing(8))
	manager.loop.mode = ModeAutopilot
	// A canceled request can finish after its replacement has begun.
	old := manager.NotifyChatActivity()
	current := manager.NotifyChatActivity()
	manager.NotifyChatActivityComplete(old)
	if manager.modeActive(ModeAutopilot) {
		t.Fatal("old request completion resumed planning during the new chat turn")
	}
	manager.NotifyChatActivityComplete(current)
	if !manager.modeActive(ModeAutopilot) {
		t.Fatal("owning request completion did not release planning")
	}
}
