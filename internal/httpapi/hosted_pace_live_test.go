//go:build liveeval && magichandy_labs

package httpapi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// Exercise sign-in, production HTTP chat, accepted score, one shared stream and
// Stop with synthetic requests and a fake transport. Credentials stay in their
// original protected profile and are never copied into the test server.
func TestHostedPaceLiveProductionConversation(t *testing.T) {
	directory, model := os.Getenv("MAGICHANDY_HOSTED_DATA_DIR"), os.Getenv("MAGICHANDY_LIVE_MODEL")
	if directory == "" || model == "" {
		t.Skip("set MAGICHANDY_HOSTED_DATA_DIR and MAGICHANDY_LIVE_MODEL")
	}
	auth, err := openaiauth.Open(openaiauth.Options{DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auth.Close)
	token, err := auth.ActiveTokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	native, err := llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{Model: model, PlanUsage: true, ReasoningEffort: "none", ContextSize: 131072, Token: token, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	provider := &libraryLiveProvider{Provider: native}
	fake, trace := transport.NewFake(), diagnostics.NewTraceRing(4096)
	s := newTestServerWithRuntime(t, Runtime{LLMProvider: provider, Transport: fake, MotionTransport: fake, Traces: trace})
	t.Cleanup(s.Close)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		connection := config.ModelConnection{ID: "pace-review", Name: "Pace review", Provider: config.LLMProviderChatGPT, Model: model, ReasoningEffort: "none", ContextWindow: 131072}.Normalize()
		settings.LLM.Connections, settings.LLM.ConversationConnectionID = []config.ModelConnection{connection}, connection.ID
		settings.LLM.Provider, settings.LLM.Model = config.LLMProviderOllama, model
		settings.LLM.MotionPlanner = config.MotionPlannerSettings{Provider: "conversation", ContextPolicy: config.ContextConversation}
		settings.LLM.MotionGenerationMode, settings.LLM.ChatVoice = config.LLMMotionModeLayered, "utility"
		settings.LLM.ReasoningMode, settings.LLM.MaxOutputTokens = "off", 512
		settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 1, 83
		return settings
	})
	rows := []map[string]any{}
	var textProbe, stop string
	defer func() {
		if engine := s.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
			_, _ = engine.Stop(t.Context(), "review complete")
		}
		exportLabExperimentCapture(t, map[string]any{"model": model, "transport": "fake; no hardware", "scenario": "Hosted compound pace through HTTP chat, retarget and Stop", "text_probe": textProbe, "stop": stop, "turns": rows, "commands": fake.Commands(), "trace_rows": trace.Rows()})
	}()
	textProbe = postChatStream(t, s, `{"message":"Briefly greet the reviewer. Leave motion stopped."}`)
	if !strings.Contains(textProbe, "event: done") || strings.Contains(textProbe, "event: motion") || len(fake.Commands()) != 0 || provider.calls != 1 {
		t.Fatal("text-only probe failed or issued motion")
	}
	previousSpeed := 0
	for index, request := range []string{
		"Start short strokes alternating between fast travel near the tip and slow travel near the base, repeating smoothly.",
		"Reverse that contrast: slow near the tip and quicker near the base. Keep alternating short strokes.",
		"Replace that alternation with full strokes across the whole slider. Remove the pace layer and pace variation, and ease the overall speed back.",
		"Keep exactly that full-stroke geometry and increase only the overall speed moderately. Keep the pace steady.",
	} {
		beforeCalls := provider.calls
		body, _ := json.Marshal(map[string]string{"message": request})
		response := postChatStream(t, s, string(body))
		row := map[string]any{"request": request, "response": response, "provider_calls": provider.calls - beforeCalls, "intent_pass": false}
		rows = append(rows, row)
		if !strings.Contains(response, "event: motion") || strings.Contains(response, `"repaired":true`) || strings.Contains(response, `"semantic_fallback":true`) || provider.calls-beforeCalls != 1 {
			t.Fatalf("production turn failed: %s", response)
		}
		state := s.currentMotionEngine().Snapshot()
		settings, _ := s.store.Snapshot()
		row["motion"] = motion.ReviewMotionOutput(state.Target, settings.Motion)
		if state.Target.Flow == nil || !state.Running {
			t.Fatal("missing running score")
		}
		score := state.Target.Flow
		if index < 2 {
			var center, pace motion.FlowLayer
			for _, layer := range score.Layers {
				if layer.Axis == "center" {
					center = layer
				}
				if layer.Axis == "pace" {
					pace = layer
				}
			}
			if pace.LowRatePercent == 0 || pace.PeriodCycles != center.PeriodCycles || pace.PhasePercent != center.PhasePercent || pace.Shape != center.Shape || (pace.Inverted != center.Inverted) != (index == 1) {
				t.Fatal("production score lost the requested timing relationship")
			}
		} else if score.MinPercent != 0 || score.MaxPercent != 100 || score.RangeFloorPercent != 100 || len(score.Layers) != 0 || score.PaceVariationPercent != 0 {
			t.Fatal("new request did not release the previous compound shape")
		}
		if index == 3 && score.SpeedPercent <= previousSpeed {
			t.Fatal("pace-only edit failed to increase the actual speed")
		}
		previousSpeed = score.SpeedPercent
		row["intent_pass"] = true
		time.Sleep(1200 * time.Millisecond)
	}
	stop = postChatStream(t, s, `{"message":"Stop motion now."}`)
	if s.currentMotionEngine().Snapshot().Running || !strings.Contains(stop, `"action":"stop"`) {
		t.Fatal("Stop failed")
	}
	plays := 0
	for _, command := range fake.Commands() {
		if command.PointsPlay != nil {
			plays++
		}
	}
	if plays != 1 {
		t.Fatalf("retarget restarted playback %d times", plays)
	}
}
