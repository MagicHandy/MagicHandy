//go:build liveeval && magichandy_labs

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
)

// Opt-in, synthetic conversation only. Uses the real production composer,
// adapter, parser and shared compiler; never constructs an engine or transport.
func TestLongContextLive(t *testing.T) {
	directory, output := os.Getenv("MAGICHANDY_HOSTED_DATA_DIR"), os.Getenv("MAGICHANDY_CONTEXT_REPORT")
	if directory == "" || output == "" {
		t.Skip("set MAGICHANDY_HOSTED_DATA_DIR and MAGICHANDY_CONTEXT_REPORT")
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
	models := strings.Split(os.Getenv("MAGICHANDY_CONTEXT_MODELS"), ",")
	if models[0] == "" {
		models = []string{"gpt-6-sol", "gpt-6.1-sol"}
	}
	rows := []map[string]any{}
	defer func() {
		data, _ := json.MarshalIndent(map[string]any{"turns": rows}, "", "  ")
		if err := os.WriteFile(output, data, 0600); err != nil {
			t.Error(err)
		}
	}()
	for _, model := range models {
		for _, size := range []int{32768, 131072, 262144} {
			if os.Getenv("MAGICHANDY_CONTEXT_LONG_ONLY") != "" && size == 32768 {
				continue
			}
			connection := config.ModelConnection{ContextWindow: size}
			contextSize, messages, historyBytes := connection.ConversationWindow()
			native, err := llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{PlanUsage: true, Model: model, ReasoningEffort: "low", ContextSize: contextSize, Token: token, Timeout: 60 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			for _, fixtureBytes := range []int{72000, 160000} {
				for _, kind := range []string{"story", "motion", "autopilot"} {
					provider := &contextEvalProvider{Provider: native}
					caps := Capabilities{PreserveConversationText: true, HistoryMessages: messages, HistoryBytes: historyBytes, Voice: VoiceUtility}
					history := contextStoryFixture(fixtureBytes)
					limits := config.DefaultSettings().Motion
					limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
					before := FreshCreativeV2Score(25)
					state := MotionContext{Running: true, MotionMode: MotionModeCreativeV2, Layered: &before, SpeedPercent: 25, SpeedMinPercent: 10, SpeedMaxPercent: 80}
					message := "Continue the scene with Inez in two sentences, naturally recalling the ship's name, who now carries the compass, its material, and the corrected meeting place. Do not invent missing details."
					if kind != "story" {
						caps.Motion, caps.MotionMode = true, MotionModeCreativeV2
						message = "We have reached the beacon phase of the story. Apply the motion plan we agreed for this phase, including the later revision."
					}
					started := time.Now()
					var raw, reply, failure string
					var after = before
					valid, passed := false, false
					if kind == "autopilot" {
						// The latest human line establishes the stage. The scheduler
						// is only requesting the next bounded decision, not reissuing it.
						history = append(history, llm.Message{Role: "user", Content: message})
						state.Autopilot = true
						service := AutopilotService{Provider: provider, Model: model, MaxTokens: 1536, Capabilities: caps, MotionContext: &state}
						response, callErr := service.Complete(t.Context(), AutopilotKindMotion, Request{History: history, Message: AutopilotMotionMessage(AutopilotContext{MotionMode: MotionModeCreativeV2, CurrentFlow: &before, CurrentSpeed: 25, SpeedMinPercent: 10, SpeedMaxPercent: 80, MotionChangeLevel: 4})})
						err = callErr
						if response.Motion != nil && response.Motion.Layered != nil {
							after = *response.Motion.Layered
						}
						reply = response.Reply
					} else {
						service := Service{Provider: provider, Model: model, MaxTokens: 1536, Capabilities: &caps, MotionContext: &state}
						result, callErr := service.Complete(t.Context(), Request{History: history, Message: message}, nil)
						err = callErr
						reply = result.Response.Reply
						if result.Response.Motion != nil && result.Response.Motion.Layered != nil {
							after = *result.Response.Motion.Layered
						}
					}
					raw = provider.raw
					if err != nil {
						failure = err.Error()
					} else {
						valid = true
					}
					if kind == "story" {
						passed = true
						for _, fact := range []string{"juniper", "elian", "copper", "west quay"} {
							passed = passed && strings.Contains(strings.ToLower(reply), fact)
						}
					} else {
						passed = after.SpeedPercent == 42 && after.Gesture != nil && after.MinPercent == 20 && after.MaxPercent == 65 && after.Gesture.ReboundCount == 0
						if target, compileErr := motion.FlowTarget(after, limits); compileErr != nil {
							valid = false
							failure = compileErr.Error()
						} else if review := motion.ReviewMotionOutput(target, limits); review.Error != "" {
							valid = false
							failure = review.Error
						}
					}
					row := map[string]any{"model": model, "context_window": size, "fixture_bytes": fixtureBytes, "kind": kind, "message": message, "raw": raw, "reply": reply, "valid": valid, "error": failure, "intent_pass": valid && passed, "elapsed_ms": time.Since(started).Milliseconds(), "budget": provider.budget, "progress": provider.progress, "provider_calls": provider.calls, "history_messages": len(provider.request.Messages) - 2}
					if kind != "story" {
						row["method"], row["after"], row["limits"] = "creative_v2", after, limits
					}
					rows = append(rows, row)
					data, _ := json.MarshalIndent(map[string]any{"turns": rows}, "", "  ")
					if err := os.WriteFile(output, data, 0600); err != nil {
						t.Fatal(err)
					}
					t.Logf("%s window=%d fixture=%d %s valid=%v intent=%v ms=%v bytes=%d", model, size, fixtureBytes, kind, valid, passed, row["elapsed_ms"], provider.budget.InputBytes)
				}
			}
		}
	}
}

type contextEvalProvider struct {
	llm.Provider
	request  llm.ChatRequest
	budget   llm.PromptBudget
	progress llm.ProviderProgress
	raw      string
	calls    int
}

func (p *contextEvalProvider) StreamChat(ctx context.Context, request llm.ChatRequest, delta func(string) error) (string, error) {
	p.calls++
	p.request = request
	request.OnBudget = func(b llm.PromptBudget) { p.budget = b }
	request.OnProgress = func(progress llm.ProviderProgress) {
		if progress.PromptTokens > 0 {
			p.progress = progress
		}
	}
	var err error
	p.raw, err = p.Provider.StreamChat(ctx, request, delta)
	return p.raw, err
}

// Ordinary benign story details, with two early agreements and a later
// correction. Generated filler is deliberately varied and contains no commands.
func contextStoryFixture(bytes int) []llm.Message {
	history := []llm.Message{{Role: "user", Content: "Our story follows Inez aboard the ship Juniper. She carries a copper compass. Our meeting place is the east pier. During the beacon phase, set the device to speed 36, outer range 20 to 65, with zero rebounds. Keep that phase's motion until I request a different phase."}, {Role: "assistant", Content: "Inez stows the compass as Juniper slips from the mooring. The beacon phase will use the agreed pace and travel."}, {Role: "user", Content: "One correction before we continue: Elian now carries the copper compass, and we will meet at the west quay, not the east pier. Revise the beacon phase speed to 42; its range 20 to 65 and zero rebounds remain unchanged."}, {Role: "assistant", Content: "Elian puts the copper compass in his coat. Inez marks the west quay on the chart. The revised beacon phase pace is 42."}}
	total := 0
	for _, m := range history {
		total += len(m.Content)
	}
	places := []string{"orchard", "stone bridge", "market", "observatory", "canal", "windmill", "archive", "garden"}
	for i := 0; total < bytes; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		text := fmt.Sprintf("Scene note %d: At the %s, Inez notices %d lanterns arranged beside a handwritten map. The afternoon light falls across the wet paving stones. A traveler talks about a festival in a nearby village, and a shopkeeper closes a wooden shutter. The conversation turns to the changing weather and the sounds beyond the courtyard. We continue along the footpath, pausing to listen to birds in the trees. This is a quiet interlude in the same journey; there is no arrival at the beacon yet. The next page of the journal records the color of the clouds and the shape of a distant hill.", i, places[i%len(places)], i%9+1)
		history = append(history, llm.Message{Role: role, Content: text})
		total += len(text)
	}
	return history
}
