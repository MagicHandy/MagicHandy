package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestAutopilotResumesAfterContinuousModeSwitch(t *testing.T) {
	for _, tc := range []struct {
		from, to, reply string
		score           motion.FlowSpec
	}{
		{config.LLMMotionModeLayered, config.LLMMotionModeCreativeV2,
			`{"edits":[{"evolve":true}],"reply":"Fresh variation."}`, chat.DefaultLayeredScore(37)},
		{config.LLMMotionModeCreativeV2, config.LLMMotionModeLayered,
			`{"edits":{"evolve":true},"reply":"Fresh variation."}`, chat.FreshCreativeV2Score(37)},
	} {
		t.Run(tc.from+"_to_"+tc.to, func(t *testing.T) {
			fake := transport.NewFake()
			provider := &scriptedLLMProvider{responses: []string{tc.reply}}
			server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
			t.Cleanup(server.Close)
			saveSettings(t, server.store, func(s config.Settings) config.Settings {
				s.LLM.MotionGenerationMode = tc.from
				return s
			})
			if _, err := server.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionStart, Layered: &tc.score}); err != nil {
				t.Fatal(err)
			}
			engine := server.currentMotionEngine()
			before := engine.Snapshot()
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, withController(httptest.NewRequest(http.MethodPut, "/api/settings/llm-motion-mode", strings.NewReader(`{"mode":"`+tc.to+`"}`))))
			if response.Code != http.StatusOK {
				t.Fatal(response.Body.String())
			}
			// The scheduler can still own the preceding mode's segment when the
			// user restarts Autopilot. That score must not become the new grammar.
			decision, err := server.autopilotDecide(t.Context(), modes.DecisionInput{
				CurrentFlow: before.Target.Flow, CurrentSpeed: 37, SpeedMinPercent: 10, SpeedMaxPercent: 43,
			})
			if err != nil || decision.Hold || decision.Segment.Flow == nil || provider.callCount() != 1 {
				t.Fatalf("mode handoff failed: %+v %v", decision, err)
			}
			if (decision.Segment.Flow.Gesture != nil) != (tc.to == config.LLMMotionModeCreativeV2) || decision.Segment.SpeedPercent != 37 {
				t.Fatal("new score has the wrong grammar or pace")
			}
			if _, err := server.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionUpdate, Layered: decision.Segment.Flow}); err != nil {
				t.Fatal(err)
			}
			if server.currentMotionEngine() != engine {
				t.Fatal("mode switch replaced the shared engine")
			}
			if _, err := server.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionStop}); err != nil || engine.Snapshot().Running {
				t.Fatalf("Stop failed after mode handoff: %v", err)
			}
		})
	}
}
