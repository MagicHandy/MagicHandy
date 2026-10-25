package chat

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestHostedRepairPreservesOriginalContractAndExactFeedback(t *testing.T) {
	bad := `{"reply":"Draft","unknown":true}`
	provider := &scriptedProvider{responses: []string{bad, `{"reply":"Corrected."}`}}
	caps := Capabilities{PreserveConversationText: true}
	service := Service{Provider: provider, Model: "hosted-model", ReasoningMode: "off", Capabilities: &caps, HostedResponseRepair: true}
	var events []string
	result, err := service.Complete(t.Context(), Request{Message: "Keep this exact request.", History: []llm.Message{{Role: "user", Content: "Earlier context."}}}, func(event StreamEvent) error {
		events = append(events, event.Type)
		return nil
	})
	if err != nil || !result.Repaired || result.Malformed || result.CanRepair() || result.SemanticFallback || result.Response.Reply != "Corrected." || len(provider.requests) != 2 {
		t.Fatalf("repair = %+v %v calls=%d", result, err, len(provider.requests))
	}
	first, second := provider.requests[0], provider.requests[1]
	if !reflect.DeepEqual(first.Messages, second.Messages[:len(first.Messages)]) || string(first.JSONSchema) != string(second.JSONSchema) ||
		second.Model != first.Model || second.ReasoningMode != first.ReasoningMode || second.PreserveTailMessages != 3 ||
		second.Messages[len(second.Messages)-2].Content != bad || !strings.Contains(second.Messages[len(second.Messages)-1].Content, "unknown") ||
		result.Raw != bad || result.RepairRaw == "" || result.InitialError == "" || result.Input == nil {
		t.Fatalf("repair lost original context or feedback: %+v", second)
	}
	if !reflect.DeepEqual(events, []string{"delta", "repair_start", "repair_delta"}) {
		t.Fatalf("repair progress = %v", events)
	}
}

func TestHostedRepairNeverRetriesRefusalProviderErrorsOrPlainProse(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		err       error
	}{
		{"refusal", "", &llm.CloudError{Kind: "refusal"}},
		{"auth", "", &llm.CloudError{Kind: "auth"}},
		{"network", `{"reply":"partial`, errors.New("network unavailable")},
		{"output cap", `{"reply":"partial`, llm.ErrOutputTruncated},
		{"unstructured prose", "An ordinary prose response.", nil},
		{"empty", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &scriptedProvider{responses: []string{tc.raw}, responseErrors: []error{tc.err}}
			caps := Capabilities{PreserveConversationText: true}
			result, err := (Service{Provider: provider, Capabilities: &caps, HostedResponseRepair: true}).Complete(t.Context(), Request{Message: "Hello."}, nil)
			if err == nil || result.CanRepair() || result.RepairAttempted || len(provider.requests) != 1 || result.Response.Motion != nil || result.Response.Reply != "" {
				t.Fatalf("ineligible response was repaired: %+v %v", result, err)
			}
		})
	}
}

func TestHostedRepairSingleUseIncludesFailedAndCanceledAttempts(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		provider := &scriptedProvider{responses: []string{`{"unknown":true}`, `{"still_wrong":true}`}}
		caps := Capabilities{PreserveConversationText: true}
		service := Service{Provider: provider, Capabilities: &caps}
		original, err := service.Complete(t.Context(), Request{Message: "Hello."}, nil)
		if err == nil || !original.CanRepair() || len(provider.requests) != 1 {
			t.Fatal("manual candidate missing", err)
		}
		ctx, finish := context.WithCancel(t.Context())
		if cancel {
			finish()
		}
		result, err := service.Repair(ctx, original, nil)
		finish()
		if err == nil || result.CanRepair() || original.CanRepair() || result.Response.Reply != "" || result.Response.Motion != nil {
			t.Fatalf("failed/canceled repair was admitted: %+v %v", result, err)
		}
		before := len(provider.requests)
		if _, err := service.Repair(t.Context(), original, nil); err == nil || len(provider.requests) != before {
			t.Fatal("original handle could be reused")
		}
	}
}

func TestHostedRepairContextBudgetFailureKeepsOriginalAndCannotFallback(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"unknown":true}`, ""}, responseErrors: []error{nil, &llm.CloudError{Kind: "context"}}}
	caps := Capabilities{PreserveConversationText: true}
	service := Service{Provider: provider, Capabilities: &caps, HostedResponseRepair: true}
	result, err := service.Complete(t.Context(), Request{Message: "Keep this exact request."}, nil)
	var outcome *llm.CloudError
	if !errors.As(err, &outcome) || outcome.Kind != "context" || len(provider.requests) != 2 || result.CanRepair() || result.Raw != `{"unknown":true}` || result.Response.Reply != "" || result.Response.Motion != nil {
		t.Fatalf("context rejection lost or bypassed: %+v %v", result, err)
	}
}

func TestHostedManualRepairConcurrentDoubleClickMakesOneRequest(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"unknown":true}`, `{"reply":"One correction."}`}}
	caps := Capabilities{PreserveConversationText: true}
	service := Service{Provider: provider, Capabilities: &caps}
	result, _ := service.Complete(t.Context(), Request{Message: "Hello."}, nil)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { _, _ = service.Repair(t.Context(), result, nil) })
	}
	wg.Wait()
	if len(provider.requests) != 2 {
		t.Fatalf("provider requests=%d", len(provider.requests))
	}
}

func TestHostedLayeredRepairUsesSameAuthorizationAndNoPartialEdits(t *testing.T) {
	for _, repaired := range []string{
		`{"action":"none","edits":{},"reply":"Kept stopped."}`,
		`{"action":"update","edits":{"controls":{"speed_percent":{"op":"set","value":30}}},"reply":"Changed."}`,
	} {
		provider := &scriptedProvider{responses: []string{`{"action":"none","edits":{"unknown":true},"reply":"Bad."}`, repaired}}
		caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
		score := DefaultLayeredScore(25)
		state := MotionContext{Running: false, MotionMode: MotionModeLayered, Layered: &score, SpeedMinPercent: 10, SpeedMaxPercent: 50}
		service := Service{Provider: provider, Capabilities: &caps, MotionContext: &state, HostedResponseRepair: true}
		result, err := service.Complete(t.Context(), Request{Message: "Discuss the current pattern without starting it."}, nil)
		if strings.Contains(repaired, `"action":"none"`) {
			if err != nil || !result.Repaired || (result.Response.Motion != nil && result.Response.Motion.Action != MotionActionNone) {
				t.Fatalf("repair=%+v %v", result, err)
			}
		} else if err == nil || result.Response.Motion != nil || result.Response.Reply != "" {
			t.Fatalf("repair bypassed stopped authority: %+v %v", result, err)
		}
		if score.SpeedPercent != 25 || len(provider.requests) != 2 {
			t.Fatal("partial edit or extra retry")
		}
	}
}

func TestHostedRepairSwitchDoesNotChangeLocalPromptsOrCalls(t *testing.T) {
	var requests [2][]llm.ChatRequest
	for i, enabled := range []bool{false, true} {
		provider := &scriptedProvider{responses: []string{`{"reply":"unfinished`, `{"reply":"Local recovery."}`}}
		service := Service{Provider: provider, HostedResponseRepair: enabled}
		result, err := service.Complete(t.Context(), Request{Message: "Hello."}, nil)
		if err != nil || !result.Repaired {
			t.Fatalf("local result=%+v %v", result, err)
		}
		requests[i] = provider.requests
	}
	if !reflect.DeepEqual(requests[0], requests[1]) {
		t.Fatal("hosted repair setting changed local prompts or generation")
	}
}
