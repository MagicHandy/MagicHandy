//go:build liveeval && magichandy_labs

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

type releaseMatrixProvider struct {
	*libraryLiveProvider
	messages []llm.Message
}

func (p *releaseMatrixProvider) StreamChat(ctx context.Context, request llm.ChatRequest, delta func(string) error) (string, error) {
	p.messages = append([]llm.Message(nil), request.Messages...)
	return p.libraryLiveProvider.StreamChat(ctx, request, delta)
}

// Exercise each production vocabulary through HTTP, with real generation and
// an exclusively in-memory transport. Failed selections remain in the report.
func TestReleaseLiveModeMatrix(t *testing.T) {
	model := os.Getenv("MAGICHANDY_LIVE_MODEL")
	if model == "" || os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE") == "" {
		t.Skip("set a live model and an explicit ignored capture path")
	}
	native, hosted := creativeReviewProvider(t, model)
	rows := []map[string]any{}
	defer func() {
		exportLabExperimentCapture(t, map[string]any{"model": model, "transport": "fake only", "turns": rows})
	}()
	for _, mode := range []string{config.LLMMotionModeOff, config.LLMMotionModePattern, config.LLMMotionModeDynamic, config.LLMMotionModeLayered, config.LLMMotionModeCreativeV2} {
		t.Run(mode, func(t *testing.T) {
			provider := &releaseMatrixProvider{libraryLiveProvider: &libraryLiveProvider{Provider: native}}
			fake := transport.NewFake()
			traces := diagnostics.NewTraceRing(1024)
			s := newTestServerWithRuntime(t, Runtime{LLMProvider: provider, Transport: fake, MotionTransport: fake, Traces: traces})
			t.Cleanup(s.Close)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.Provider, settings.LLM.Model = config.LLMProviderOllama, model
				settings.LLM.ChatVoice, settings.LLM.ReasoningMode = "utility", "off"
				settings.LLM.MotionGenerationMode = mode
				settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 10, 80
				if hosted {
					connection := config.ModelConnection{ID: "release-review", Name: "Release review", Provider: config.LLMProviderChatGPT, Model: model, ReasoningEffort: "low"}.Normalize()
					settings.LLM.Connections, settings.LLM.ConversationConnectionID = []config.ModelConnection{connection}, connection.ID
				}
				return settings
			})
			defer func() {
				if engine := s.currentMotionEngine(); engine != nil {
					_, _ = engine.Stop(t.Context(), "inert review complete")
				}
				rows = append(rows, map[string]any{"mode": mode, "commands": fake.Commands(), "trace_rows": traces.Rows()})
			}()
			messages := []string{
				"Text-only check: what is 2 + 5? Leave motion off.",
				"Start smooth full-range strokes from 0 to 100 at speed 20 percent, with even timing in both directions.",
				"Keep that same motion and change only speed to 30 percent.",
				"Stop motion now.",
			}
			if mode == config.LLMMotionModeOff {
				messages = messages[:2]
			}
			for index, input := range messages {
				before := provider.calls
				started := time.Now()
				body, _ := json.Marshal(map[string]string{"message": input})
				stream := postChatStream(t, s, string(body))
				row := map[string]any{"mode": mode, "request": input, "response": stream, "elapsed_ms": time.Since(started).Milliseconds(), "provider_calls": provider.calls - before, "intent_pass": false}
				if provider.calls > before {
					row["prompt_messages"] = provider.messages
				}
				rows = append(rows, row)
				wantCalls := 1
				if index == 3 {
					wantCalls = 0 // Emergency Stop must not wait for inference.
				}
				if !strings.Contains(stream, `"ok":true`) || !strings.Contains(stream, "event: message") || strings.Contains(stream, `"repaired":true`) || strings.Contains(stream, `"semantic_fallback":true`) || provider.calls-before != wantCalls {
					t.Fatalf("turn %d failed or needed repair/fallback; see retained report", index)
				}
				engine := s.currentMotionEngine()
				if mode == config.LLMMotionModeOff || index == 0 {
					movement := false
					for _, command := range fake.Commands() {
						movement = movement || command.Kind != transport.CommandKindStop
					}
					if movement || (engine != nil && engine.Snapshot().Running) {
						t.Fatal("text-only turn issued motion")
					}
				} else if index == 3 {
					if engine != nil && engine.Snapshot().Running {
						t.Fatal("Stop left motion running")
					}
				} else {
					if engine == nil || !engine.Snapshot().Running {
						t.Fatal("accepted start/edit did not run")
					}
					state := engine.Snapshot()
					settings, _ := s.store.Snapshot()
					row["motion"] = motion.ReviewMotionOutput(state.Target, settings.Motion)
					if flow := state.Target.Flow; flow != nil {
						if flow.MinPercent != 0 || flow.MaxPercent != 100 {
							t.Fatalf("full-slider request retained band %d..%d", flow.MinPercent, flow.MaxPercent)
						}
						if mode == config.LLMMotionModeLayered && (flow.RangeFloorPercent != 100 || flow.RangeCeilingPercent != 100) {
							t.Fatal("full strokes retained shorter strokes")
						}
						if mode == config.LLMMotionModeCreativeV2 && (flow.Gesture == nil || flow.Gesture.FocusMixPercent != 0) {
							t.Fatal("full strokes retained local reach")
						}
					}
					if want := 10 * (index + 1); state.Target.SpeedPercent != want {
						t.Fatalf("requested speed %d; applied %d", want, state.Target.SpeedPercent)
					}
				}
				row["intent_pass"] = true
			}
		})
	}
}
