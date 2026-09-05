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
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestCreativeV2LiveProductionConversation(t *testing.T) {
	model := os.Getenv("MAGICHANDY_LIVE_MODEL")
	if model == "" {
		t.Skip("set MAGICHANDY_LIVE_MODEL")
	}
	native, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	provider := &libraryLiveProvider{Provider: native}
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(4096)
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider, Transport: fake, MotionTransport: fake, Traces: traces})
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(s config.Settings) config.Settings {
		s.LLM.Provider = config.LLMProviderOllama
		s.LLM.Model = model
		s.LLM.ReasoningMode = "off"
		s.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		s.Motion.SpeedMinPercent, s.Motion.SpeedMaxPercent = 20, 80
		return s
	})
	outputs := []map[string]any{}
	defer func() {
		if server.currentMotionEngine().Snapshot().Running {
			_, _ = server.currentMotionEngine().Stop(t.Context(), "live evaluation complete")
		}
		exportLabExperimentCapture(t, map[string]any{"model": model, "transport": "captured fake transport; no physical device", "scenario": "Live Creative v2 chat, same-stream retargets, Autopilot generation and Stop", "turns": outputs, "commands": fake.Commands(), "trace_rows": traces.Rows()})
	}()
	for index, message := range []string{
		"Start moving with only short strokes at the tip. Travel faster toward the tip and return more slowly.",
		"Add inertia so the velocity crest builds later within each stroke. Keep the pace and reach.",
		"Mix full strokes with shrinking rebounds at the base. Use local width 45 and three rebounds retaining 75 percent each.",
		"Remove the bounce and preserve everything else.",
		"Keep varying within this same character.",
	} {
		beforeCalls := provider.calls
		body, _ := json.Marshal(map[string]string{"message": message})
		response := postChatStream(t, server, string(body))
		output := map[string]any{"request": message, "response": response, "provider_calls": provider.calls - beforeCalls}
		output["intent_pass"] = true
		outputs = append(outputs, output)
		if !strings.Contains(response, "event: motion") || strings.Contains(response, `"repaired":true`) || strings.Contains(response, `"semantic_fallback":true`) || provider.calls-beforeCalls != 1 {
			output["intent_pass"] = false
			t.Fatalf("live turn failed: %s", response)
		}
		state := server.currentMotionEngine().Snapshot()
		s := state.Target.Flow
		if s == nil || s.Gesture == nil || !state.Running {
			t.Fatal("missing Creative v2 target")
		}
		g := s.Gesture
		settings, _ := server.store.Snapshot()
		output["motion"] = motion.ReviewMotionOutput(state.Target, settings.Motion)
		if index == 0 && (g.FocusPercent != 100 || g.FocusMixPercent != 100 || g.FasterDirection != "tip" || g.ContrastPercent <= 0) {
			output["intent_pass"] = false
			t.Errorf("wrong sweep: %+v", g)
		}
		if index == 1 && g.InertiaPercent <= 25 {
			output["intent_pass"] = false
			t.Error("inertia did not increase")
		}
		if index == 2 && (g.FocusPercent != 0 || g.FocusMixPercent == 0 || g.FocusMixPercent == 100 || g.ReboundCount != 3 || g.ReboundDecayPercent != 75 || g.FocusWidthPercent != 45) {
			output["intent_pass"] = false
			t.Errorf("wrong rebounds: %+v", g)
		}
		if index >= 3 && g.ReboundCount != 0 {
			output["intent_pass"] = false
			t.Error("rebounds not removed")
		}
		time.Sleep(1200 * time.Millisecond)
	}
	state := server.currentMotionEngine().Snapshot()
	beforeCalls := provider.calls
	decision, err := server.autopilotDecide(t.Context(), modes.DecisionInput{CurrentFlow: state.Target.Flow, CurrentSpeed: state.Target.SpeedPercent, SpeedMinPercent: 20, SpeedMaxPercent: 80})
	if err != nil || decision.Hold || decision.Segment.Flow == nil || decision.Segment.Flow.Seed == state.Target.Flow.Seed || provider.calls-beforeCalls != 1 {
		t.Fatalf("Autopilot: %+v %v", decision, err)
	}
	settings, _ := server.store.Snapshot()
	outputs = append(outputs, map[string]any{"request": "Production Autopilot continuation (generated, not scheduled in this fixture)", "motion": motion.ReviewMotionOutput(decision.Segment.Target("Creative v2", "autopilot"), settings.Motion), "provider_calls": provider.calls - beforeCalls})
	stop := postChatStream(t, server, `{"message":"Stop motion now."}`)
	if server.currentMotionEngine().Snapshot().Running || !strings.Contains(stop, `"action":"stop"`) {
		t.Fatalf("Stop: %s", stop)
	}
	plays := 0
	for _, command := range fake.Commands() {
		if command.PointsPlay != nil {
			plays++
		}
	}
	if plays != 1 {
		t.Fatalf("unexpected transport restart count %d", plays)
	}
	t.Logf("%s: five production edits, one Autopilot generation, one transport play, stopped", model)
}
