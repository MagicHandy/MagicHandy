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
)

// All output goes to the in-process Buttplug fixture, never physical hardware.
func TestManualVibrationResumeProductionChatLive(t *testing.T) {
	if os.Getenv("MAGICHANDY_RESUME_EVAL") != "1" {
		t.Skip("opt-in model evaluation with simulated outputs")
	}
	provider, model, hosted := timingLiveProvider(t)
	simple := !hosted && os.Getenv("MAGICHANDY_RESUME_LEVEL") == config.ModelCapabilitySimple
	if !hosted && !simple {
		var err error
		provider, err = llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: os.Getenv("MAGICHANDY_LIVE_LLAMA_URL"), Model: model, ContextSize: 32768, Timeout: 2 * time.Minute})
		if err != nil {
			t.Fatal(err)
		}
	}
	capture := &vibrationAppCapture{Provider: provider}
	s := independentVibrationServer(t)
	if simple && os.Getenv("MAGICHANDY_SIMPLE_RESUME_CUE") != "" {
		capture.Provider = &resumeStateTrialProvider{Provider: capture.Provider, stopped: func() bool {
			engine := s.currentMotionEngine()
			return engine == nil || !engine.Snapshot().Running
		}}
	}
	s.llm.provider, s.traces = capture, diagnostics.NewTraceRing(4096)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Model, settings.LLM.ReasoningMode = model, config.LLMReasoningOff
		if !hosted {
			settings.LLM.Provider, settings.LLM.ConversationConnectionID = config.LLMProviderOllama, "local"
			settings.LLM.LocalCapabilityLevel = config.ModelCapabilityIntermediate
			settings.LLM.LlamaCPPContextSize, settings.LLM.MaxOutputTokens = 32768, 512
			if simple {
				settings.LLM.Provider, settings.LLM.LlamaCPPMode = config.LLMProviderLlamaCPP, config.LlamaCPPModeManaged
				settings.LLM.LocalCapabilityLevel = config.ModelCapabilitySimple
				settings.LLM.LlamaCPPContextSize, settings.LLM.MaxOutputTokens = 16384, 256
			}
		}
		return settings
	})
	rows := []map[string]any{}
	t.Cleanup(func() {
		if path := os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE"); path != "" {
			data, _ := json.MarshalIndent(map[string]any{"model": model, "hosted": hosted, "transport": "in-process Buttplug; no hardware", "turns": rows, "trace": s.traces.Rows()}, "", "  ")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Error(err)
			}
		}
	})
	for _, onlyVibrator := range []bool{false, true} {
		if onlyVibrator {
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.Device = settings.Device.WithPlaybackDevices([]string{config.DeviceVibrator})
				return settings
			})
		}
		for i, message := range []string{"What is the current playback state?", "Resume with a gently varying rhythm.", "What does the intensity ceiling do?", "Start again with a different flowing rhythm."} {
			if i == 0 || i == 2 {
				stopVibrationForResume(t, s)
			}
			calls, started := capture.calls, time.Now()
			body, _ := json.Marshal(map[string]string{"message": message})
			response := postChatStream(t, s, string(body))
			var after motion.ActiveMotionState
			if e := s.currentMotionEngine(); e != nil {
				after = e.Snapshot()
			}
			v := s.intifaceSnapshot().Vibrator
			wantMotion := i%2 == 1
			pass := !strings.Contains(response, "event: error") && !strings.Contains(response, `"repaired":true`) && !strings.Contains(response, `"semantic_fallback":true`) && capture.calls-calls == 1 && after.Running == wantMotion
			if wantMotion {
				pass = pass && v.IntensityPercent == 40
			} else {
				pass = pass && v.IntensityPercent == 0
			}
			// A successful answer with no motion is still a valid response.
			valid := !strings.Contains(response, "event: error") && strings.Contains(response, "event: message")
			rows = append(rows, map[string]any{"model": model, "valid": valid, "method": "layered", "recipe_name": "manual_resume", "message": message, "raw": capture.raw, "provider_request": capture.lastRequest, "sse": response, "intent_pass": pass, "after": after.Target.Flow, "engine_after": after, "limits": config.DefaultSettings().Motion, "vibration": after.Target.Vibration, "only_vibrator": onlyVibrator, "provider_calls": capture.calls - calls, "elapsed_ms": time.Since(started).Milliseconds(), "vibrator": v})
			if !pass {
				t.Errorf("only_vibrator=%v turn=%d failed; capture retains response", onlyVibrator, i+1)
			}
		}
		_, _ = s.emergencyStop(t.Context(), "live_resume_finished")
		// Explicitly arm the same simulated selection for the next scenario.
		_, _ = vibratorRequest(t, s, "/api/transport/intiface/vibrator/intensity", `{"intensity_percent":40}`, true)
	}
}

// Test-only transfer of the successful Intermediate state cue to Simple.
type resumeStateTrialProvider struct {
	llm.Provider
	stopped func() bool
}

func (p *resumeStateTrialProvider) StreamChat(ctx context.Context, input llm.ChatRequest, emit func(string) error) (string, error) {
	if p.stopped() && len(input.Messages) > 0 {
		last := len(input.Messages) - 1
		messages := append([]llm.Message(nil), input.Messages[:last]...)
		cue := "Current app state for this turn: playback is stopped, not paused. Previous chat describes earlier runs. A new motion request may start the saved or revised rhythm; a question leaves playback stopped."
		if os.Getenv("MAGICHANDY_SIMPLE_RESUME_CUE") == "full" {
			cue += " Vibration follows the shared motion within the intensity set by the user; its ceiling caps vibration, not stroke speed. You cannot change that setting."
		}
		input.Messages = append(messages, llm.Message{Role: "system", Content: cue}, input.Messages[last])
	}
	return p.Provider.StreamChat(ctx, input, emit)
}
