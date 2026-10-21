//go:build liveeval && magichandy_labs

package chat

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
)

func TestLongContextFlowLive(t *testing.T) {
	directory, output := os.Getenv("MAGICHANDY_HOSTED_DATA_DIR"), os.Getenv("MAGICHANDY_CONTEXT_REPORT")
	if directory == "" || output == "" {
		t.Skip("explicit hosted profile and report required")
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
	rows := []map[string]any{}
	defer func() {
		data, _ := json.MarshalIndent(map[string]any{"turns": rows}, "", "  ")
		if err := os.WriteFile(output, data, 0600); err != nil {
			t.Error(err)
		}
	}()
	for _, model := range []string{"gpt-6-sol", "gpt-6.1-sol"} {
		native, err := llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{PlanUsage: true, Model: model, ReasoningEffort: "low", ContextSize: 131072, Token: token, Timeout: 60 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		catalog, err := native.Models(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range catalog {
			if item.Slug == model {
				t.Logf("model=%s advertised_context=%d", model, item.ContextWindow)
			}
		}
		provider := &contextEvalProvider{Provider: native}
		history := contextStoryFixture(72000)
		caps := Capabilities{PreserveConversationText: true, HistoryMessages: 256, HistoryBytes: 96000, Voice: VoiceUtility, Motion: true, MotionMode: MotionModeCreativeV2}
		limits := config.DefaultSettings().Motion
		limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
		score := FreshCreativeV2Score(25)
		state := MotionContext{Running: true, MotionMode: MotionModeCreativeV2, Layered: &score, SpeedPercent: 25, SpeedMinPercent: 10, SpeedMaxPercent: 80}
		messages := []string{
			"Continue our story in two sentences without recapping. Introduce a ferry captain named Neri. Leave motion unchanged.",
			"Inez asks Neri whether the storm will clear. Continue their conversation in two sentences. Leave motion unchanged.",
			"Elian hands the copper compass to Neri to keep. Continue the scene in two sentences. Leave motion unchanged.",
			"Return to our original ship and our corrected meeting place, naming both naturally in two sentences. Who now has the compass? Leave motion unchanged.",
			"We've reached the beacon phase. Apply the motion we agreed for this stage, including its later revision, while continuing the scene.",
			"Revise only the speed to 30 percent and preserve the travel range and character. Continue the scene briefly.",
			"Continue the scene with Neri showing the compass. This is conversation only; leave the device paused.",
			"Finish this chapter at our agreed meeting place, naming it and the ship. This is conversation only; the device must remain stopped.",
		}
		for i, message := range messages {
			state.Paused = i == 6
			state.Running = i != 7
			state.Layered = &score
			state.SpeedPercent = score.SpeedPercent
			beforeCalls := provider.calls
			started := time.Now()
			service := Service{Provider: provider, Model: model, MaxTokens: 1536, Capabilities: &caps, MotionContext: &state}
			result, callErr := service.Complete(t.Context(), Request{Message: message, History: history}, nil)
			passed := callErr == nil && provider.calls-beforeCalls == 1 && !result.Repaired && !result.SemanticFallback
			if i == 4 || i == 5 {
				if result.Response.Motion == nil || result.Response.Motion.Layered == nil {
					passed = false
				} else {
					score = *result.Response.Motion.Layered
					want := 42
					if i == 5 {
						want = 30
					}
					passed = passed && score.SpeedPercent == want && score.MinPercent == 20 && score.MaxPercent == 65
				}
			} else {
				passed = passed && result.Response.Motion == nil
			}
			lower := strings.ToLower(result.Response.Reply)
			if i == 3 {
				passed = passed && strings.Contains(lower, "juniper") && strings.Contains(lower, "west quay") && strings.Contains(lower, "neri")
			}
			if i == 7 {
				passed = passed && strings.Contains(lower, "juniper") && strings.Contains(lower, "west quay")
			}
			errorText := ""
			if callErr != nil {
				errorText = callErr.Error()
			}
			target, compileErr := motion.FlowTarget(score, limits)
			if compileErr != nil {
				passed = false
				errorText = compileErr.Error()
			} else if review := motion.ReviewMotionOutput(target, limits); review.Error != "" {
				passed = false
				errorText = review.Error
			}
			rows = append(rows, map[string]any{"model": model, "case": i, "message": message, "reply": result.Response.Reply, "raw": result.Raw, "valid": callErr == nil, "intent_pass": passed, "error": errorText, "elapsed_ms": time.Since(started).Milliseconds(), "budget": provider.budget, "progress": provider.progress, "method": "creative_v2", "after": score, "limits": limits, "paused": state.Paused, "running": state.Running})
			history = append(history, llm.Message{Role: "user", Content: message}, llm.Message{Role: "assistant", Content: result.Response.Reply})
			t.Logf("%s turn=%d pass=%v ms=%d cached=%d", model, i, passed, time.Since(started).Milliseconds(), provider.progress.CachedPromptTokens)
		}
	}
}
