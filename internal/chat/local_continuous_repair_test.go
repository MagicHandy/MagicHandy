package chat

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func localRepairService(provider llm.Provider, mode MotionMode) Service {
	caps := Capabilities{Motion: true, MotionMode: mode}
	before := DefaultLayeredScore(40)
	if mode == MotionModeCreativeV2 {
		before = FreshCreativeV2Score(40)
	}
	before.Seed = 981
	return Service{Provider: provider, Model: "local-test", ReasoningMode: "auto", ReasoningBudgetTokens: 512,
		MaxTokens: 768, Capabilities: &caps, PromptBudget: PromptBudgetSettings{ContextSize: 32768},
		MotionContext: &MotionContext{Running: true, MotionMode: mode, Layered: &before, SpeedMinPercent: 1, SpeedMaxPercent: 100}}
}

func TestLocalContinuousRepairRegeneratesOriginalRequestWithoutInvalidDraft(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		t.Run(string(mode), func(t *testing.T) {
			bad := `{"action":"update","edits":{"unknown":1},"reply":"INVALID_DRAFT_MUST_NOT_BECOME_CONTEXT"}`
			good := `{"action":"update","edits":{"change_by":{"speed_percent":5}},"reply":"Five points faster."}`
			if mode == MotionModeCreativeV2 {
				good = `{"action":"update","edits":[{"speed_percent":45}],"reply":"Five points faster."}`
			}
			provider := &scriptedProvider{responses: []string{bad, good}}
			service := localRepairService(provider, mode)
			var events []string
			result, err := service.Complete(t.Context(), Request{Message: "Increase speed by five points; keep the other controls.",
				History: []llm.Message{{Role: "user", Content: "An earlier conversation."}, {Role: "assistant", Content: "Earlier reply."}}}, func(event StreamEvent) error {
				events = append(events, event.Type)
				return nil
			})
			if err != nil || !result.Repaired || !result.RepairAttempted || result.Malformed || result.SemanticFallback || result.CanRepair() ||
				result.Raw != bad || result.RepairRaw != good || result.Input == nil || result.InitialError == "" || len(provider.requests) != 2 {
				t.Fatalf("repair=%+v err=%v calls=%d", result, err, len(provider.requests))
			}
			assertLocalCorrectionRequest(t, provider.requests[0], provider.requests[1], result.InitialError)
			if !reflect.DeepEqual(events, []string{"delta", "local_repair_start", "repair_delta"}) {
				t.Fatalf("progress lost: %v", events)
			}
			if result.Response.Motion == nil || result.Response.Motion.Layered.SpeedPercent != 45 || service.MotionContext.Layered.SpeedPercent != 40 {
				t.Fatal("repair was not transactional")
			}
		})
	}
}

func assertLocalCorrectionRequest(t *testing.T, first, repair llm.ChatRequest, initialError string) {
	t.Helper()
	if !reflect.DeepEqual(first.Messages, repair.Messages[:len(first.Messages)]) || string(first.JSONSchema) != string(repair.JSONSchema) ||
		repair.Model != first.Model || repair.MaxTokens != first.MaxTokens || repair.PreserveTailMessages != 2 || repair.ReasoningMode != "off" || repair.ReasoningBudgetTokens != 0 ||
		repair.Temperature != 0 || repair.TopP != 0 || repair.RepeatPenalty != 0 || repair.RepeatLastN != 0 {
		t.Fatalf("repair changed original contract or bounded controls: %+v", repair)
	}
	for _, message := range repair.Messages {
		if strings.Contains(message.Content, "INVALID_DRAFT_MUST_NOT_BECOME_CONTEXT") {
			t.Fatal("invalid draft was fed back to the local model")
		}
	}
	_, encoded, _ := strings.Cut(repair.Messages[len(repair.Messages)-1].Content, "\n")
	var feedback map[string]string
	if json.Unmarshal([]byte(encoded), &feedback) != nil || feedback["validator_error"] != initialError {
		t.Fatalf("validator feedback lost: %+v", feedback)
	}
}

func TestLocalContinuousRepairNoExtraCallsForSuccessOrIneligibleOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		err       error
		single    bool
		autopilot bool
	}{
		{name: "success", raw: `{"action":"none","edits":{},"reply":"Explanation only."}`},
		{name: "refusal", err: &llm.CloudError{Kind: "refusal"}},
		{name: "auth", err: &llm.CloudError{Kind: "auth"}},
		{name: "network", raw: `{"unfinished":`, err: errors.New("network failed")},
		{name: "truncation", raw: `{"unfinished":`, err: llm.ErrOutputTruncated},
		{name: "unstructured refusal", raw: "I cannot help with that request."},
		{name: "empty"},
		{name: "bounded retry", raw: `{"unknown":true}`, single: true},
		{name: "autopilot", raw: `{"unknown":true}`, autopilot: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &scriptedProvider{responses: []string{tc.raw}, responseErrors: []error{tc.err}}
			service := localRepairService(provider, MotionModeLayered)
			service.SingleAttempt, service.TrustedMotionInput = tc.single, tc.autopilot
			result, err := service.Complete(t.Context(), Request{Message: "Explain this control."}, nil)
			if len(provider.requests) != 1 || result.RepairAttempted || result.Repaired || result.Response.Motion != nil || (err == nil) != (tc.name == "success") {
				t.Fatalf("unexpected retry/effects: calls=%d result=%+v err=%v", len(provider.requests), result, err)
			}
		})
	}
}

func TestLocalContinuousRepairRejectsFailedSecondResponseWithoutPartialEffects(t *testing.T) {
	for _, responseErr := range []error{nil, &llm.CloudError{Kind: "refusal"}, errors.New("network unavailable"), llm.ErrOutputTruncated} {
		provider := &scriptedProvider{responses: []string{`{"unknown":true}`, `{"action":"update","edits":{"controls":{"speed_percent":55},"unknown":true},"reply":"Partial claim."}`}, responseErrors: []error{nil, responseErr}}
		service := localRepairService(provider, MotionModeLayered)
		result, err := service.Complete(t.Context(), Request{Message: "Set speed to 55."}, nil)
		if err == nil || len(provider.requests) != 2 || !result.RepairAttempted || result.Repaired || result.CanRepair() || result.Response.Motion != nil || result.Response.Reply != "" ||
			result.RepairRaw == "" || result.InitialError == "" || service.MotionContext.Layered.SpeedPercent != 40 {
			t.Fatalf("failed repair leaked effects: %+v %v", result, err)
		}
	}
}

func TestLocalContinuousRepairPreservesPausedAndQuestionAuthority(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, paused := range []bool{false, true} {
			empty := `{}`
			if mode == MotionModeCreativeV2 {
				empty = `[]`
			}
			good := `{"action":"none","edits":` + empty + `,"reply":"Motion is unchanged."}`
			provider := &scriptedProvider{responses: []string{`{"unknown":true}`, good}}
			service := localRepairService(provider, mode)
			service.MotionContext.Paused = paused
			result, err := service.Complete(t.Context(), Request{Message: "Only explain the current settings."}, nil)
			if err != nil || !result.Repaired || result.Response.Motion != nil || len(provider.requests) != 2 {
				t.Fatalf("no-motion repair=%+v err=%v", result, err)
			}
		}
	}
	provider := &scriptedProvider{responses: []string{`{"unknown":true}`, `{"action":"update","edits":{"controls":{"speed_percent":50}},"reply":"Changing."}`}}
	service := localRepairService(provider, MotionModeLayered)
	service.MotionContext.Paused = true
	result, err := service.Complete(t.Context(), Request{Message: "Change the speed."}, nil)
	if err == nil || result.Response.Motion != nil || result.Response.Reply != "" || service.MotionContext.Layered.SpeedPercent != 40 {
		t.Fatalf("paused repair gained authority: %+v %v", result, err)
	}
}

func TestLocalContinuousRepairCancellationBeforeAndDuringReplacement(t *testing.T) {
	for _, phase := range []string{"local_repair_start", "repair_delta"} {
		ctx, cancel := context.WithCancel(t.Context())
		provider := &scriptedProvider{responses: []string{`{"unknown":true}`, `{"action":"update","edits":{"controls":{"speed_percent":50}},"reply":"Changed."}`}}
		service := localRepairService(provider, MotionModeLayered)
		result, err := service.Complete(ctx, Request{Message: "Set speed to 50."}, func(event StreamEvent) error {
			if event.Type == phase {
				cancel()
				return ctx.Err()
			}
			return nil
		})
		cancel()
		if !errors.Is(err, context.Canceled) || result.Response.Motion != nil || result.Response.Reply != "" || result.InitialError == "" {
			t.Fatalf("cancellation leaked effects: %+v %v", result, err)
		}
		want := 1
		if phase == "repair_delta" {
			want = 2
		}
		if len(provider.requests) != want {
			t.Fatalf("calls=%d want=%d", len(provider.requests), want)
		}
	}
}

func TestLocalContinuousRepairBudgetPreservesLatestUserOrSkipsCall(t *testing.T) {
	input := llm.ChatRequest{Messages: []llm.Message{{Role: "system", Content: "The original contract and profile."},
		{Role: "user", Content: strings.Repeat("old ", 5000)}, {Role: "assistant", Content: "Old reply."},
		{Role: "user", Content: "The exact latest request."}}, MaxTokens: 512, Model: "same-model"}
	provider := &scriptedProvider{responses: []string{`{"reply":"Corrected."}`}}
	service := Service{Provider: provider, PromptBudget: PromptBudgetSettings{ContextSize: 16384}}
	parse := func(string) (AssistantResponse, error) { return AssistantResponse{Reply: "Corrected."}, nil }
	result, err := service.repairLocalContinuous(t.Context(), input, `{"unknown":true}`, errors.New("unknown field"), parse, nil)
	if err != nil || !result.Repaired || len(provider.requests) != 1 {
		t.Fatalf("budgeted repair=%+v %v", result, err)
	}
	repair := provider.requests[0]
	if repair.Messages[len(repair.Messages)-2].Content != "The exact latest request." || repair.HistoryMessagesDropped == 0 || repair.Messages[0] != input.Messages[0] {
		t.Fatalf("budget lost original intent or profile: %+v", repair)
	}
	input.Messages[0].Content = strings.Repeat("essential profile ", 1000)
	provider.requests = nil
	result, err = service.repairLocalContinuous(t.Context(), input, `{"unknown":true}`, errors.New("original validation failure"), parse, nil)
	if err == nil || len(provider.requests) != 0 || result.RepairAttempted || result.Raw != `{"unknown":true}` || result.InitialError != "original validation failure" || result.MalformedError != result.InitialError || result.Response.Motion != nil {
		t.Fatalf("over-budget repair lost rejection or made a call: %+v %v", result, err)
	}
}

func TestLocalContinuousRepairOverBudgetKeepsOriginalSingleCall(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"unknown":true}`}}
	service := localRepairService(provider, MotionModeCreativeV2)
	service.SingleAttempt, service.ReasoningMode, service.ReasoningBudgetTokens = true, "off", 0
	service.PromptBudget.ContextSize = 16384
	message := "Keep every current control."
	_, _ = service.Complete(t.Context(), Request{Message: message}, nil)
	_, budget, err := llm.BudgetChatRequest(provider.requests[0], 16384)
	if err != nil {
		t.Fatal("budget fixture already exceeds the initial budget", err)
	}
	// Leave room for the original request, but not a complete feedback message.
	message += strings.Repeat("x", budget.LimitBytes-budget.InputBytes-100)
	provider.responses, provider.requests = []string{`{"unknown":true}`}, nil
	service.SingleAttempt = false
	result, err := service.Complete(t.Context(), Request{Message: message}, nil)
	if err == nil || len(provider.requests) != 1 || result.RepairAttempted || result.InitialError == "" || result.MalformedError != result.InitialError || result.Raw != `{"unknown":true}` {
		t.Fatalf("over-budget correction lost initial rejection: %+v %v calls=%d", result, err, len(provider.requests))
	}
	if _, _, err := llm.BudgetChatRequest(provider.requests[0], 16384); err != nil {
		t.Fatal("original request must fit before checking repair reserve", err)
	}
}
