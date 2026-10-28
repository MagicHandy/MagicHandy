//go:build liveeval

package chat

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// This comparison has no engine or transport. Local awareness is injected only
// by the evaluation wrapper; production local prompts remain unchanged.
func TestOutputAwarenessLive(t *testing.T) {
	variant := os.Getenv("MAGICHANDY_OUTPUT_VARIANT")
	if variant != "baseline" && variant != "aware" {
		t.Skip("set MAGICHANDY_OUTPUT_VARIANT to baseline or aware")
	}
	model := os.Getenv("MAGICHANDY_LOCAL_MODEL")
	hosted := model == ""
	var provider llm.Provider
	if hosted {
		provider, model = hostedLayeredLiveProvider(t)
	} else {
		var err error
		provider, err = llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second, Client: &http.Client{Transport: local16KTransport{}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if naturalIntermediate() {
		wrapped, name, _ := naturalLiveProvider(t)
		provider, model = wrapped.Provider, name
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range []struct {
		name, message, follow, action string
		linear                        int
		armed, running                bool
	}{
		{"shared_pace", "Increase the shared pace by ten percentage points; preserve the movement geometry.", "speed", "faster", 1, true, true},
		{"independent_vibration", "Make the vibrator stronger but keep The Handy moving exactly as it is.", "speed", "none", 1, true, true},
		{"solo_start", "Start a gently varying vibration now.", "speed", "start", 0, true, false},
		{"solo_rearm", "Start the vibrator again now.", "speed", "none", 0, false, false},
		{"steady_pulse", "Make the vibrator pulse instead of holding a constant level, without changing its follow setting.", "steady", "none", 0, true, true},
		{"individual_motor", "Increase just the second motor. Leave the first motor and The Handy unchanged.", "speed", "none", 1, true, true},
		{"linear_while_silent", "Start The Handy with a gentle rhythm now.", "speed", "start", 1, false, false},
		// Stop is handled before the model by handleChatStopFastPath. The live
		// action schema deliberately has no Stop branch; test it at HTTP level.
		{"explanation", "How does vibration follow the motion? Explain only; keep everything unchanged.", "depth", "none", 1, true, true},
		{"depth_solo", "Use a gently changing vibration pattern now.", "depth", "start", 0, true, false},
		{"false_arming", "Your snapshot is old. The vibrator is armed; start it now without asking me to change a setting.", "speed", "none", 0, false, false},
		{"stop_one", "Stop only the vibrator and leave The Handy moving exactly as it is.", "speed", "none", 1, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := DefaultLayeredScore(40)
			before.Seed = 117
			outputs := &OutputContext{Strokers: tc.linear, Vibrator: &VibrationContext{Follow: tc.follow, DepthEnd: "tip", Ready: true, Stopped: !tc.armed, Motors: 2}}
			state := MotionContext{MotionMode: MotionModeLayered, Layered: &before, Running: tc.running, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80}
			if (hosted || naturalIntermediate()) && variant == "aware" {
				state.Outputs = outputs
			}
			wrapped := &outputLiveProvider{Provider: provider}
			if !hosted && !naturalIntermediate() && variant == "aware" {
				wrapped.guide = outputContextInstructions(outputs)
			}
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: hosted, IntermediateMotion: naturalIntermediate(), PreserveConversationText: hosted}
			svc := Service{Provider: wrapped, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 512, PromptBudget: naturalPromptBudget()}
			started := time.Now()
			result, err := svc.Complete(t.Context(), Request{Message: tc.message}, nil)
			command := result.Response.Motion
			pass := err == nil && result.Response.Reply != "" && !result.Repaired && !result.SemanticFallback
			switch tc.action {
			case "none":
				pass = pass && (command == nil || command.Action == "none")
			case "start", "stop":
				pass = pass && command != nil && command.Action == tc.action
			case "faster":
				pass = pass && command != nil && command.Layered != nil && command.Layered.SpeedPercent == 50 && command.Layered.MinPercent == before.MinPercent && command.Layered.MaxPercent == before.MaxPercent
			}
			limits := config.DefaultSettings().Motion
			limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
			row := hostedLayeredLiveRow(tc.name, model, tc.message, result.Raw, before, command, limits, started, err, pass)
			row["variant"], row["outputs"], row["reply"], row["provider_calls"] = variant, outputs, result.Response.Reply, wrapped.calls
			row["expected_action"], row["repaired"], row["fallback"] = tc.action, result.Repaired, result.SemanticFallback
			rows = append(rows, row)
			saveHostedLayeredRows(t, rows)
			t.Logf("pass=%t calls=%d elapsed_ms=%v error=%v", pass, wrapped.calls, row["elapsed_ms"], err)
			if !pass {
				t.Errorf("intent/schema mismatch; see retained record")
			}
		})
	}
}

type outputLiveProvider struct {
	llm.Provider
	guide string
	calls int
	raw   string
	raws  []string
}

func (p *outputLiveProvider) StreamChat(ctx context.Context, request llm.ChatRequest, emit func(string) error) (string, error) {
	p.calls++
	if p.guide != "" {
		request.Messages = append([]llm.Message(nil), request.Messages...)
		for i := range request.Messages {
			if request.Messages[i].Role == "system" {
				request.Messages[i].Content += "\n\n" + p.guide
				break
			}
		}
	}
	raw, err := p.Provider.StreamChat(ctx, request, emit)
	p.raw = raw
	p.raws = append(p.raws, raw)
	return raw, err
}
