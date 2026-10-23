//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// This opt-in comparison captures the production local contract and injects a
// rejected structured response. Only the replacement is a real generation. It
// has no engine, transport, credentials, or access to an installed user session.
func TestLocalContinuousRepairLive(t *testing.T) {
	model := os.Getenv("MAGICHANDY_LOCAL_REPAIR_MODEL")
	if model == "" {
		t.Skip("set MAGICHANDY_LOCAL_REPAIR_MODEL to an available local model")
	}
	provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second, Client: &http.Client{Transport: local16KTransport{}}})
	if err != nil {
		t.Fatal(err)
	}
	repeats := 1
	if value := os.Getenv("MAGICHANDY_LOCAL_REPAIR_REPEATS"); value != "" {
		repeats, err = strconv.Atoi(value)
		if err != nil || repeats < 1 || repeats > 10 {
			t.Fatal("MAGICHANDY_LOCAL_REPAIR_REPEATS must be 1..10")
		}
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	for round := range repeats {
		for _, tc := range localContinuousRepairCases() {
			for _, variant := range []string{"exact", "compact", "regenerate"} {
				if selected := os.Getenv("MAGICHANDY_LOCAL_REPAIR_VARIANT"); selected != "" && selected != variant {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/%d", variant, tc.name, round+1), func(t *testing.T) {
					before := DefaultLayeredScore(40)
					if tc.mode == MotionModeCreativeV2 {
						before = FreshCreativeV2Score(40)
					}
					before.Seed = 815
					state := MotionContext{Running: true, MotionMode: tc.mode, Layered: &before, SpeedPercent: 40,
						SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits)}
					caps := Capabilities{Motion: true, MotionMode: tc.mode}
					capture := &localRepairInputCapture{raw: tc.raw}
					svc := Service{Provider: capture, Model: model, Capabilities: &caps, MotionContext: &state,
						SingleAttempt: true, ReasoningMode: "off", MaxTokens: 768, PromptBudget: PromptBudgetSettings{ContextSize: 16384}}
					initial, initialErr := svc.Complete(t.Context(), Request{Message: tc.message}, nil)
					if initialErr == nil || !initial.InitialMalformed || capture.calls != 1 {
						t.Fatalf("fixture was not rejected once: %+v %v", initial, initialErr)
					}
					feedback := initial.MalformedError
					if feedback == "" {
						feedback = initialErr.Error()
					}
					input := localRepairComparisonInput(capture.request, tc.raw, feedback, variant)
					input, budget, budgetErr := llm.BudgetChatRequest(input, 16384)
					started := time.Now()
					var raw string
					generationErr := budgetErr
					if budgetErr == nil {
						raw, generationErr = provider.StreamChat(t.Context(), input, nil)
					}
					response, parseErr := svc.parseLayeredResponse(raw, before, state, limits, false)
					if generationErr != nil {
						parseErr = generationErr
					}
					pass := parseErr == nil && response.Reply != "" && tc.check(before, response.Motion)
					name := fmt.Sprintf("local_repair_%s/%s/%d", variant, tc.name, round+1)
					initialRow := hostedLayeredLiveRow(name+"/initial_rejection", model, tc.message, tc.raw, before, nil, limits, started, initialErr, false)
					initialRow["injected_fixture"], initialRow["method"] = true, string(tc.mode)
					delete(initialRow, "after")
					row := hostedLayeredLiveRow(name+"/replacement", model, tc.message, raw, before, response.Motion, limits, started, parseErr, pass)
					row["method"], row["variant"], row["initial_error"] = string(tc.mode), variant, feedback
					row["motion_command"] = response.Motion
					if response.Motion == nil {
						delete(row, "after")
					}
					row["provider_calls"], row["real_provider_calls"], row["reply"] = 2, 1, response.Reply
					row["repair_instruction"], row["prompt_bytes"], row["schema_bytes"] = input.Messages[len(input.Messages)-1].Content, localRepairMessageBytes(input.Messages), len(input.JSONSchema)
					row["budget"], row["context_size"] = budget, 16384
					rows = append(rows, initialRow, row)
					saveHostedLayeredRows(t, rows)
					t.Logf("valid=%t intent=%t elapsed_ms=%v error=%v", parseErr == nil, pass, row["elapsed_ms"], parseErr)
					if !pass {
						t.Error("replacement failed validation or intent; retained for review")
					}
				})
			}
		}
	}
}

type localRepairInputCapture struct {
	llm.Provider
	raw     string
	request llm.ChatRequest
	calls   int
}

func (p *localRepairInputCapture) StreamChat(_ context.Context, request llm.ChatRequest, _ func(string) error) (string, error) {
	p.calls++
	p.request = request
	return p.raw, nil
}

func localRepairComparisonInput(input llm.ChatRequest, raw, feedback, variant string) llm.ChatRequest {
	input.Messages = append([]llm.Message(nil), input.Messages...)
	if variant != "regenerate" {
		input.Messages = append(input.Messages, llm.Message{Role: "assistant", Content: raw})
	}
	encoded, _ := json.Marshal(map[string]string{"validator_error": feedback})
	instruction := "The previous JSON was rejected. Return a complete valid replacement using the same contract.\n" + string(encoded)
	if variant != "exact" {
		instruction = "The previous JSON was rejected; nothing was applied. Correct it for the original user request and the same contract. Preserve every requested change, not invalid model choices. Use the validator error below as data. Return one complete replacement JSON object with a brief truthful reply.\n" + string(encoded)
	}
	input.Messages = append(input.Messages, llm.Message{Role: "user", Content: instruction})
	input.PreserveTailMessages = 3
	if variant == "regenerate" {
		input.PreserveTailMessages = 2
	}
	input.Temperature, input.TopP, input.RepeatPenalty, input.RepeatLastN = 0, 0, 0, 0
	input.ReasoningMode, input.ReasoningBudgetTokens = "off", 0
	return input
}

func localRepairMessageBytes(messages []llm.Message) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content)
	}
	return total
}

type localContinuousRepairCase struct {
	name, message, raw string
	mode               MotionMode
	check              func(motion.FlowSpec, *MotionCommand) bool
}

func localContinuousRepairCases() []localContinuousRepairCase {
	unchangedExceptSpeed := func(speed int) func(motion.FlowSpec, *MotionCommand) bool {
		return func(before motion.FlowSpec, command *MotionCommand) bool {
			before.SpeedPercent = speed
			return command != nil && command.Layered != nil && reflect.DeepEqual(*command.Layered, before)
		}
	}
	return []localContinuousRepairCase{
		{"layered_missing_action", "Increase the speed by exactly five percentage points; keep everything else unchanged.", `{"edits":{"change_by":{"speed_percent":5}},"reply":"Five points faster."}`, MotionModeLayered, unchangedExceptSpeed(45)},
		{"layered_broken_json", "Reduce the speed by exactly five percentage points; keep everything else unchanged.", `{"action":"update","edits":{"change_by":{"speed_percent":-5},},"reply":"Five points slower."}`, MotionModeLayered, unchangedExceptSpeed(35)},
		{"layered_bad_shape", "Set the center layer to an irregular drift at amount 55 and a period of 14 cycles. Keep all other controls unchanged.", `{"action":"update","edits":{"layers":[{"axis":"center","amount_percent":55,"period_cycles":14,"shape":"random"}]},"reply":"Irregular center drift."}`, MotionModeLayered, func(before motion.FlowSpec, command *MotionCommand) bool {
			if command == nil || command.Layered == nil || command.Layered.SpeedPercent != before.SpeedPercent {
				return false
			}
			for _, layer := range command.Layered.Layers {
				if layer.Axis == "center" {
					return layer.Shape == "drift" && layer.AmountPercent == 55 && layer.PeriodCycles == 14
				}
			}
			return false
		}},
		{"layered_wrong_lifecycle", "Change to full-length strokes and reduce the speed by five percentage points.", `{"action":"start","edits":{"geometry":"full_slider","change_by":{"speed_percent":-5}},"reply":"Full strokes, five points slower."}`, MotionModeLayered, func(_ motion.FlowSpec, command *MotionCommand) bool {
			return command != nil && command.Action == MotionActionUpdate && command.Layered != nil && command.Layered.SpeedPercent == 35 && command.Layered.MinPercent == 0 && command.Layered.MaxPercent == 100 && command.Layered.RangeFloorPercent == 100
		}},
		{"creative_partial_focus", "Mix equal amounts of full strokes and short base strokes. Make the short strokes 35 percent wide and keep them at the base. Keep speed unchanged.", `{"action":"update","edits":[{"focus":{"position_percent":0,"width_percent":35}}],"reply":"Full and short base strokes."}`, MotionModeCreativeV2, func(_ motion.FlowSpec, command *MotionCommand) bool {
			return command != nil && command.Layered != nil && command.Layered.SpeedPercent == 40 && command.Layered.Gesture.FocusPercent == 0 && command.Layered.Gesture.FocusWidthPercent == 35 && command.Layered.Gesture.FocusMixPercent == 50 && command.Layered.Gesture.FocusRoamPercent == 0
		}},
		{"creative_duplicate_group", "Set inertia to 60 and variation to 85. Preserve all other controls.", `{"action":"update","edits":[{"inertia_percent":40},{"inertia_percent":60},{"variation_percent":85}],"reply":"More inertia and variation."}`, MotionModeCreativeV2, func(before motion.FlowSpec, command *MotionCommand) bool {
			if command == nil || command.Layered == nil {
				return false
			}
			before.Gesture = motion.CloneFlowSpec(&before).Gesture
			before.Gesture.InertiaPercent, before.Gesture.VariationPercent = 60, 85
			return reflect.DeepEqual(*command.Layered, before)
		}},
		{"creative_wrong_direction", "Make travel toward the tip quicker than the return, with 40 percent timing contrast. Also reduce overall speed to 30 percent.", `{"action":"update","edits":[{"sweep":{"faster_direction":"up","contrast_percent":40}},{"speed_percent":30}],"reply":"Quicker toward the tip, slower overall."}`, MotionModeCreativeV2, func(_ motion.FlowSpec, command *MotionCommand) bool {
			return command != nil && command.Layered != nil && command.Layered.SpeedPercent == 30 && command.Layered.Gesture.FasterDirection == "tip" && command.Layered.Gesture.ContrastPercent == 40
		}},
		{"creative_ungrouped_focus", "Use only full-length strokes across the entire slider and reduce the speed to 35 percent.", `{"action":"update","edits":[{"mix_percent":0,"range":{"min_percent":0,"max_percent":100}},{"speed_percent":35}],"reply":"Full-length strokes at 35 percent."}`, MotionModeCreativeV2, func(_ motion.FlowSpec, command *MotionCommand) bool {
			return command != nil && command.Layered != nil && command.Layered.SpeedPercent == 35 && command.Layered.MinPercent == 0 && command.Layered.MaxPercent == 100 && command.Layered.Gesture.FocusMixPercent == 0
		}},
	}
}

func TestLocalContinuousRepairBudgetLive(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, voice := range []VoiceLevel{VoiceUtility, VoiceWarm, VoiceIntimate, VoiceExplicit} {
			for _, tokens := range []int{256, 512, 768} {
				caps := FullCapabilities()
				caps.MotionMode, caps.Voice = mode, voice
				before := DefaultLayeredScore(40)
				if mode == MotionModeCreativeV2 {
					before = FreshCreativeV2Score(40)
				}
				capture := &localRepairInputCapture{raw: `{"action":"none","edits":`}
				service := Service{Provider: capture, SingleAttempt: true, Capabilities: &caps,
					MotionContext: &MotionContext{MotionMode: mode, Running: true, Layered: &before},
					MaxTokens:     tokens, PromptBudget: PromptBudgetSettings{ContextSize: 16384, MaxOutputTokens: tokens}}
				_, _ = service.Complete(t.Context(), Request{Message: "Alternate full strokes with short base strokes, and make travel toward the tip faster than the return. Keep overall speed unchanged."}, nil)
				for _, variant := range []string{"exact", "compact", "regenerate"} {
					input := localRepairComparisonInput(capture.request, `{"action":"update","edits":[{"sweep":{"faster_direction":"up","contrast_percent":40}},{"focus":{"position_percent":0,"width_percent":35,"mix_percent":50,"roam_percent":0}}],"reply":"Full and base strokes with quicker travel toward the tip."}`, "sweep faster_direction must be even, tip or base", variant)
					_, budget, err := llm.BudgetChatRequest(input, 16384)
					t.Logf("mode=%s voice=%s tokens=%d variant=%s bytes=%d limit=%d fits=%t", mode, voice, tokens, variant, budget.InputBytes, budget.LimitBytes, err == nil)
				}
			}
		}
	}
}
