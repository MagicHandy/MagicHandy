package chat

import (
	"strings"
	"testing"
)

func TestDynamicContractExamplesMatchLifecycle(t *testing.T) {
	caps := FullCapabilities()
	caps.MotionMode = MotionModeDynamic
	for _, state := range []MotionContext{{}, {Running: true}, {Running: true, Paused: true}} {
		contract := contractForMotionState(caps, &state)
		if strings.Contains(contract, `"action":"start"`) != (!state.Running && !state.Paused) ||
			strings.Contains(contract, `"action":"update"`) != (state.Running && !state.Paused) ||
			!strings.Contains(contract, `"action":"none"`) || !strings.Contains(contract, `"action":"stop"`) {
			t.Fatalf("invalid action examples for %+v", state)
		}
	}
	if contractForMotionState(caps, nil) != contractInstructions(caps) {
		t.Fatal("state-free contract changed")
	}
}
