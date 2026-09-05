//go:build liveeval

package httpapi

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

var errLiveGeometry = errors.New("unexpected live geometry")

func TestLayeredLiveProductionConversationAndAutopilot(t *testing.T) {
	model := os.Getenv("MAGICHANDY_LIVE_MODEL")
	if model == "" {
		t.Skip("set MAGICHANDY_LIVE_MODEL to an installed model")
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
		s.LLM.MotionGenerationMode = config.LLMMotionModeLayered
		s.Motion.SpeedMinPercent, s.Motion.SpeedMaxPercent = 20, 80
		return s
	})
	outputs := []map[string]any{}
	defer func() {
		exportLabExperimentCapture(t, map[string]any{"model": model, "transport": "captured fake transport; no physical device", "scenario": "Live production Layered conversation, shared-engine retargets, Autopilot generation and Stop", "turns": outputs, "commands": fake.Commands(), "trace_rows": traces.Rows()})
	}()
	for index, message := range []string{"Start moving, with short strokes alternating between the two ends.", "Keep the current motion but make it gentler.", "Alternate long full strokes with short ones anchored at the tip.", "Keep varying the motion while preserving this character."} {
		beforeCalls := provider.calls
		body, _ := json.Marshal(map[string]string{"message": message})
		response := postChatStream(t, server, string(body))
		output := map[string]any{"request": message, "response": response, "provider_calls": provider.calls - beforeCalls}
		outputs = append(outputs, output)
		if !strings.Contains(response, "event: motion") || strings.Contains(response, `"repaired":true`) || strings.Contains(response, `"semantic_fallback":true`) || provider.calls-beforeCalls != 1 {
			t.Fatalf("live turn failed: %s", response)
		}
		state := server.currentMotionEngine().Snapshot()
		spec := state.Target.Flow
		if spec == nil || !state.Running {
			t.Fatal("missing Layered engine score")
		}
		if index == 0 && validateLiveGeometry(*spec, "alternate_ends") != nil {
			t.Fatal("wrong initial geometry")
		}
		if index == 1 && spec.SpeedPercent != 20 {
			t.Fatal("gentler did not lower speed to 20")
		}
		if index >= 2 && validateLiveGeometry(*spec, "full_and_tip") != nil {
			t.Fatal("wrong broad/tip geometry")
		}
		settings, _ := server.store.Snapshot()
		output["motion"] = motion.ReviewMotionOutput(state.Target, settings.Motion)
		time.Sleep(1200 * time.Millisecond)
	}
	state := server.currentMotionEngine().Snapshot()
	beforeCalls := provider.calls
	decision, err := server.autopilotDecide(t.Context(), modes.DecisionInput{CurrentFlow: state.Target.Flow, CurrentSpeed: state.Target.SpeedPercent, SpeedMinPercent: 20, SpeedMaxPercent: 80})
	if err != nil || decision.Hold || decision.Segment.Flow == nil || decision.Segment.Flow.Seed == state.Target.Flow.Seed || provider.calls-beforeCalls != 1 {
		t.Fatalf("live Autopilot failed: %+v %v", decision, err)
	}
	settings, _ := server.store.Snapshot()
	outputs = append(outputs, map[string]any{"request": "Production Autopilot continuation", "motion": motion.ReviewMotionOutput(decision.Segment.Target("Layered", "autopilot"), settings.Motion), "provider_calls": provider.calls - beforeCalls})
	stop := postChatStream(t, server, `{"message":"Stop motion now."}`)
	if server.currentMotionEngine().Snapshot().Running || !strings.Contains(stop, `"action":"stop"`) {
		t.Fatalf("Stop running=%v response=%s", server.currentMotionEngine().Snapshot().Running, stop)
	}
	plays := 0
	for _, c := range fake.Commands() {
		if c.PointsPlay != nil {
			plays++
		}
	}
	if plays != 1 {
		t.Fatalf("unexpected stream restart: %d", plays)
	}
	t.Logf("%s: four live production edits, one Autopilot generation, one transport play, stopped", model)
}

func validateLiveGeometry(spec motion.FlowSpec, geometry string) error {
	raw, _ := json.Marshal(map[string]any{"edits": map[string]string{"geometry": geometry}, "reply": "Expected geometry"})
	_, expected, _, err := chat.ParseLayeredReply(string(raw), spec, config.DefaultSettings().Motion)
	if err != nil {
		return err
	}
	if expected.RangeFloorPercent != spec.RangeFloorPercent || expected.RangeCeilingPercent != spec.RangeCeilingPercent || expected.AnchorPercent != spec.AnchorPercent {
		return errLiveGeometry
	}
	for _, layer := range expected.Layers {
		found := false
		for _, actual := range spec.Layers {
			if layer.Axis == actual.Axis && layer.AmountPercent == actual.AmountPercent && layer.Shape == actual.Shape {
				found = true
			}
		}
		if !found {
			return errLiveGeometry
		}
	}
	return nil
}
