package httpapi

import (
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestCreativeV2ProductionRetargetModeFenceAutopilotAndStop(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"edits":[{"focus":{"position_percent":0,"width_percent":45,"mix_percent":55}},{"rebounds":{"count":3,"retained_width_percent":75}}],"reply":"Starting base rebounds and full strokes."}`,
		`{"edits":[{"inertia_percent":70}],"reply":"A later velocity crest."}`,
		`{"edits":[{"evolve":true}],"reply":"Fresh variation."}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(s config.Settings) config.Settings {
		s.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		return s
	})
	for _, message := range []string{`{"message":"Start moving with base rebounds and full strokes."}`, `{"message":"Add inertia to build momentum later within each stroke."}`} {
		response := postChatStream(t, server, message)
		if !strings.Contains(response, "event: motion") {
			t.Fatalf("missing motion: %s", response)
		}
	}
	engine := server.currentMotionEngine()
	state := engine.Snapshot()
	if state.Target.Flow == nil || state.Target.Flow.Gesture == nil || state.Target.Label != "Creative v2" || state.Target.Flow.Gesture.InertiaPercent != 70 {
		t.Fatal("wrong shared-engine target")
	}
	state.Target.Flow.Gesture.InertiaPercent = 0
	if engine.Snapshot().Target.Flow.Gesture.InertiaPercent != 70 {
		t.Fatal("aliased live gesture")
	}
	state = engine.Snapshot()
	decision, err := server.autopilotDecide(t.Context(), modes.DecisionInput{CurrentFlow: state.Target.Flow, CurrentSpeed: 25, SpeedMinPercent: 1, SpeedMaxPercent: 100})
	if err != nil || decision.Hold || decision.Segment.Flow == nil || decision.Segment.Flow.Seed == state.Target.Flow.Seed || *decision.Segment.Flow.Gesture != *state.Target.Flow.Gesture {
		t.Fatalf("Autopilot: %+v %v", decision, err)
	}
	for _, mode := range []string{config.LLMMotionModeLayered, config.LLMMotionModeDynamic, config.LLMMotionModePattern, config.LLMMotionModeOff} {
		saveSettings(t, server.store, func(s config.Settings) config.Settings { s.LLM.MotionGenerationMode = mode; return s })
		if _, err := server.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionUpdate, Layered: motion.CloneFlowSpec(state.Target.Flow)}); err == nil {
			t.Fatal("late response crossed mode switch")
		}
	}
	stop := postChatStream(t, server, `{"message":"Stop motion now."}`)
	if engine.Snapshot().Running || !strings.Contains(stop, `"action":"stop"`) {
		t.Fatalf("Stop failed: %s", stop)
	}
	plays := 0
	for _, command := range fake.Commands() {
		if command.PointsPlay != nil {
			plays++
		}
	}
	if plays != 1 {
		t.Fatalf("retarget restarted transport %d times", plays)
	}
}
