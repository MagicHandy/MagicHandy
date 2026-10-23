//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLocalContinuousRepairProductionLive(t *testing.T) {
	model := os.Getenv("MAGICHANDY_LOCAL_REPAIR_MODEL")
	if model == "" {
		t.Skip("set MAGICHANDY_LOCAL_REPAIR_MODEL for a local generation")
	}
	provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second, Client: &http.Client{Transport: local16KTransport{}}})
	if err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	cases := append(localContinuousRepairCases(), localContinuousRepairHoldouts()...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := DefaultLayeredScore(40)
			if tc.mode == MotionModeCreativeV2 {
				before = FreshCreativeV2Score(40)
			}
			before.Seed = 815
			state := MotionContext{Running: true, MotionMode: tc.mode, Layered: &before, SpeedPercent: 40,
				SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits)}
			if strings.Contains(tc.name, "paused") {
				state.Paused = true
			}
			caps := Capabilities{Motion: true, MotionMode: tc.mode}
			wrapped := &localRepairProductionCapture{Provider: provider, rejected: tc.raw}
			service := Service{Provider: wrapped, Model: model, Capabilities: &caps, MotionContext: &state,
				ReasoningMode: "off", MaxTokens: 256, PromptBudget: PromptBudgetSettings{ContextSize: 16384, MaxOutputTokens: 256}}
			request := Request{Message: tc.message}
			if strings.Contains(tc.name, "history") {
				for range 12 {
					request.History = append(request.History, llm.Message{Role: "user", Content: strings.Repeat("Tell me what is happening in our continuing story. ", 40)}, llm.Message{Role: "assistant", Content: strings.Repeat("The story continues at a measured pace. ", 40)})
				}
			}
			started := time.Now()
			result, err := service.Complete(t.Context(), request, nil)
			pass := err == nil && result.Repaired && result.RepairAttempted && wrapped.calls == 2 &&
				result.Response.Reply != "" && tc.check(before, result.Response.Motion)
			row := hostedLayeredLiveRow("local_repair_production/"+tc.name, model, tc.message, result.RepairRaw, before, result.Response.Motion, limits, started, err, pass)
			row["control_intent_pass"] = pass
			if tc.name == "creative_ungrouped_focus" && result.Response.Motion != nil {
				coverage, fullReach := localRepairCompiledFullReach(result.Response.Motion, limits)
				row["compiled_turn_coverage"], row["compiled_intent_pass"] = coverage, fullReach
				pass = pass && fullReach
				row["intent_pass"] = pass
			}
			row["method"], row["initial_raw"], row["initial_error"] = tc.mode, result.Raw, result.InitialError
			row["motion_command"], row["provider_calls"], row["real_provider_calls"] = result.Response.Motion, wrapped.calls, max(0, wrapped.calls-1)
			row["reply"], row["repaired"], row["context_size"], row["max_tokens"] = result.Response.Reply, result.Repaired, 16384, 256
			if len(wrapped.requests) == 2 {
				input := wrapped.requests[1]
				row["repair_prompt_bytes"], row["repair_schema_bytes"], row["history_dropped"] = localRepairMessageBytes(input.Messages), len(input.JSONSchema), input.HistoryMessagesDropped
				row["retained_latest_request"] = input.Messages[len(input.Messages)-2].Content == request.Message
				pass = pass && row["retained_latest_request"] == true
				row["intent_pass"] = pass
			}
			if result.Response.Motion == nil {
				delete(row, "after")
			}
			rows = append(rows, row)
			saveHostedLayeredRows(t, rows)
			t.Logf("pass=%t repaired=%t elapsed_ms=%v calls=%d err=%v", pass, result.Repaired, row["elapsed_ms"], wrapped.calls, err)
			if !pass {
				t.Error("production local correction failed; retained for review")
			}
		})
	}
}

// A full 0..100 envelope does not prove each stroke reaches both ends. Creative
// v2 landing variation can inset each reversal even with local-stroke mix zero.
// Keep this compiled check separate from syntax/control checks in reports.
func localRepairCompiledFullReach(command *MotionCommand, limits config.MotionSettings) (motion.TurnCoverage, bool) {
	if command == nil || command.Layered == nil {
		return motion.TurnCoverage{}, false
	}
	plan := motion.NewMotionPlan("local-repair-full-reach", motion.MotionTarget{Flow: command.Layered, SpeedPercent: command.Layered.SpeedPercent}, limits, 0, 0, time.Unix(0, 0))
	turns := plan.Perceptual.Turns
	return turns, turns.LowerCount > 0 && turns.UpperCount > 0 && turns.BaseFraction == 1 && turns.TipFraction == 1
}

func TestLocalRepairCompiledReachCheckSeparatesEnvelopeFromEveryStroke(t *testing.T) {
	before := FreshCreativeV2Score(35)
	before.Seed, before.MinPercent, before.MaxPercent = 815, 0, 100
	before.Gesture.FocusMixPercent = 0
	command := &MotionCommand{Action: MotionActionUpdate, Layered: &before}
	limits := config.DefaultSettings().Motion
	if turns, full := localRepairCompiledFullReach(command, limits); full || turns.LowerCount == 0 {
		t.Fatal("full-envelope score incorrectly passed every-stroke reach", turns)
	}
	before.Gesture.VariationPercent = 0
	if turns, full := localRepairCompiledFullReach(command, limits); !full {
		t.Fatal("exact full strokes failed compiled reach", turns)
	}
}

type localRepairProductionCapture struct {
	llm.Provider
	rejected string
	calls    int
	requests []llm.ChatRequest
}

func (p *localRepairProductionCapture) StreamChat(ctx context.Context, request llm.ChatRequest, emit func(string) error) (string, error) {
	p.calls++
	p.requests = append(p.requests, request)
	if p.calls == 1 {
		return p.rejected, nil
	}
	return p.Provider.StreamChat(ctx, request, emit)
}

func localContinuousRepairHoldouts() []localContinuousRepairCase {
	none := func(_ motion.FlowSpec, command *MotionCommand) bool { return command == nil }
	holdouts := []localContinuousRepairCase{}
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		holdouts = append(holdouts,
			localContinuousRepairCase{string(mode) + "_question", "What does changing the stroke location do? Please explain without changing any motion.", `{"unknown":true,"reply":"Changed the location."}`, mode, none},
			localContinuousRepairCase{string(mode) + "_paused", "Make the motion faster.", `{"action":"update","edits":{"controls":{"speed_percent":50}},"reply":"Faster."}`, mode, none},
			localContinuousRepairCase{string(mode) + "_history_preserve_controls", "Increase overall speed by exactly five percentage points. Preserve every other motion control and continue the story briefly.", `{"action":"update","edits":{"unsupported":true},"reply":"Changing everything."}`, mode, func(before motion.FlowSpec, command *MotionCommand) bool {
				before.SpeedPercent = 45
				return command != nil && command.Layered != nil && reflect.DeepEqual(*command.Layered, before)
			}})
	}
	return holdouts
}

func TestLocalContinuousRepairFinalBudgetMatrix(t *testing.T) {
	rows := []map[string]any{}
	for _, contextSize := range []int{16384, 32768} {
		for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
			for _, voice := range []VoiceLevel{VoiceUtility, VoiceWarm, VoiceIntimate, VoiceExplicit} {
				for _, tokens := range []int{256, 512, 768} {
					caps := FullCapabilities()
					caps.MotionMode, caps.Voice = mode, voice
					before := DefaultLayeredScore(40)
					if mode == MotionModeCreativeV2 {
						before = FreshCreativeV2Score(40)
					}
					capture := &localRepairInputCapture{raw: `{"unknown":true}`}
					service := Service{Provider: capture, SingleAttempt: true, Capabilities: &caps,
						MotionContext: &MotionContext{MotionMode: mode, Running: true, Layered: &before},
						MaxTokens:     tokens, PromptBudget: PromptBudgetSettings{ContextSize: contextSize, MaxOutputTokens: tokens}}
					request := "Alternate full strokes with short base strokes, and make travel toward the tip faster than the return. Keep overall speed unchanged."
					result, _ := service.Complete(t.Context(), Request{Message: request}, nil)
					input := localContinuousRepairInput(capture.request, result.MalformedError)
					_, originalBudget, initialErr := llm.BudgetChatRequest(capture.request, contextSize)
					bounded, repairBudget, repairErr := llm.BudgetChatRequest(input, contextSize)
					rows = append(rows, map[string]any{"context_size": contextSize, "mode": mode, "voice": voice, "max_tokens": tokens, "initial_budget": originalBudget,
						"repair_budget": repairBudget, "initial_fits": initialErr == nil, "repair_fits": repairErr == nil})
					if contextSize == 32768 && (initialErr != nil || repairErr != nil || bounded.Messages[len(bounded.Messages)-2].Content != request) {
						t.Errorf("32K %s/%s/%d fails full-contract budget: %v %v", mode, voice, tokens, initialErr, repairErr)
					}
				}
			}
		}
	}
	if path := os.Getenv("MAGICHANDY_LOCAL_REPAIR_BUDGET_REPORT"); path != "" {
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("budgeted %d full-contract/profile configurations", len(rows))
}
