//go:build liveeval

package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type libraryLiveProvider struct {
	llm.Provider
	calls int
}

func (p *libraryLiveProvider) StreamChat(ctx context.Context, request llm.ChatRequest, delta func(string) error) (string, error) {
	p.calls++
	return p.Provider.StreamChat(ctx, request, delta)
}

func TestContinuousLibraryLiveAppToCapturedTransport(t *testing.T) {
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
	traces := diagnostics.NewTraceRing(512)
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider, Traces: traces})
	_, _, err = server.store.Update(func(settings config.Settings) (config.Settings, error) {
		settings.LLM.Provider = config.LLMProviderOllama
		settings.LLM.Model = model
		settings.LLM.OllamaBaseURL = "http://127.0.0.1:11434"
		settings.LLM.MaxOutputTokens = 768
		settings.LLM.ReasoningMode = "off"
		settings.LLM.MotionGenerationMode = config.LLMMotionModePattern
		settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 10, 43
		return settings, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	outputs := []map[string]any{}
	for _, test := range []struct {
		message string
		id      motion.PatternID
		speed   int
	}{
		{"Start full sweeps at 25 percent.", motion.PatternFullSweeps, 25},
		{"Change the movement: vary stroke length while always returning to the tip. Keep the full outer range available and keep speed 25.", "flow-tip-anchored", 25},
		{"Keep that exact movement shape and change only speed to 30 percent.", "flow-tip-anchored", 30},
	} {
		before := provider.calls
		body, _ := json.Marshal(map[string]string{"message": test.message})
		response := postChatStream(t, server, string(body))
		if !strings.Contains(response, "event: motion") || strings.Contains(response, `"repaired":true`) || strings.Contains(response, `"semantic_fallback":true`) || provider.calls-before != 1 {
			t.Fatalf("app turn did not complete without repair: %s", response)
		}
		engine := server.currentMotionEngine()
		if engine == nil {
			t.Fatal("missing shared engine")
		}
		state := engine.Snapshot()
		if !state.Running || state.Target.PatternID != test.id || state.Target.SpeedPercent != test.speed {
			t.Fatalf("wrong applied target: %+v", state.Target)
		}
		settings, _ := server.store.Snapshot()
		outputs = append(outputs, map[string]any{"request": test.message, "response": response, "motion": motion.ReviewMotionOutput(state.Target, settings.Motion), "provider_calls": provider.calls - before})
	}
	stop := postChatStream(t, server, `{"message":"Stop motion now."}`)
	if server.currentMotionEngine().Snapshot().Running {
		t.Fatal("Stop did not stop the shared engine")
	}
	plays, adds := 0, 0
	for _, command := range fake.Commands() {
		if command.Kind == transport.CommandKindPointsPlay {
			plays++
		}
		if command.Kind == transport.CommandKindPointsAdd {
			adds++
		}
	}
	if plays != 1 || adds < 1 {
		t.Fatalf("shared output restarted or lacked points: plays=%d adds=%d", plays, adds)
	}
	if path := os.Getenv("MAGICHANDY_LAB_REPORT"); path != "" {
		data, _ := json.MarshalIndent(map[string]any{"model": model, "transport": "captured fake transport; no physical device", "turns": outputs, "stop": stop, "commands": fake.Commands(), "trace_rows": traces.Rows()}, "", "  ")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("live app path: %d model calls, %d append commands, %d play, stopped", provider.calls, adds, plays)
}
