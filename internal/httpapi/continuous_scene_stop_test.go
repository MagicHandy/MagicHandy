package httpapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestAdvancedSceneStopUsesSharedEngineAndAllowsFreshStart(t *testing.T) {
	for _, mode := range []string{config.LLMMotionModeLayered, config.LLMMotionModeCreativeV2} {
		t.Run(mode, func(t *testing.T) {
			empty := `{}`
			if mode == config.LLMMotionModeCreativeV2 {
				empty = `[]`
			}
			provider := &scriptedLLMProvider{responses: []string{
				fmt.Sprintf(`{"action":"start","edits":%s,"reply":"Starting."}`, empty),
				fmt.Sprintf(`{"action":"stop","edits":%s,"reply":"Playback is stopped."}`, empty),
				fmt.Sprintf(`{"action":"start","edits":%s,"reply":"Starting again."}`, empty),
			}}
			fake := transport.NewFake()
			server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
			t.Cleanup(server.Close)
			saveSettings(t, server.store, func(s config.Settings) config.Settings {
				s.LLM.MotionGenerationMode = mode
				s.LLM.LocalCapabilityLevel = config.ModelCapabilityAdvanced
				s.LLM.LlamaCPPContextSize = 65536
				s.LLM.MaxOutputTokens = 1024
				return s
			})
			started := postChatStream(t, server, `{"message":"All set. You can begin now."}`)
			engine := server.currentMotionEngine()
			if engine == nil || !engine.Snapshot().Running {
				t.Fatalf("missing initial start: %s", started)
			}
			// Deliberately not a phrase recognized by the existing fast Stop path:
			// this reply must come from the selected model and its action contract.
			stopped := postChatStream(t, server, `{"message":"I am done for now. Leave the device completely still."}`)
			if !strings.Contains(stopped, `"action":"stop"`) || engine.Snapshot().Running || len(provider.requests) != 2 {
				t.Fatalf("model-selected Stop failed: %s", stopped)
			}
			if !strings.Contains(stopped, `"ok":true`) || !strings.Contains(stopped, "Playback is stopped.") || strings.Contains(stopped, `"repaired":true`) {
				t.Fatalf("Stop did not publish a successful uncorrected reply: %s", stopped)
			}
			commands := fake.Commands()
			if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
				t.Fatal("model Stop did not reach the shared transport")
			}
			restarted := postChatStream(t, server, `{"message":"I am ready again. Begin the saved motion."}`)
			if !engine.Snapshot().Running || engine != server.currentMotionEngine() || len(provider.requests) != 3 {
				t.Fatalf("fresh request did not resume through the same engine: %s", restarted)
			}
		})
	}
}
