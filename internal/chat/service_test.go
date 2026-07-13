package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type scriptedProvider struct {
	responses      []string
	responseErrors []error
	requests       []llm.ChatRequest
}

func (p *scriptedProvider) StreamChat(_ context.Context, request llm.ChatRequest, onDelta func(string) error) (string, error) {
	p.requests = append(p.requests, request)
	if len(p.responses) == 0 {
		return "", errors.New("scripted response missing")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	var responseErr error
	if len(p.responseErrors) > 0 {
		responseErr = p.responseErrors[0]
		p.responseErrors = p.responseErrors[1:]
	}
	if onDelta != nil {
		if err := onDelta(response); err != nil {
			return response, err
		}
	}
	return response, responseErr
}

func (p *scriptedProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Provider: "scripted", Available: true}
}

func TestParseAssistantResponseRejectsUnknownKeys(t *testing.T) {
	_, err := ParseAssistantResponse(`{"reply":"ok","transport":{"path":"hsp/stop"}}`)
	if err == nil {
		t.Fatal("expected strict parser to reject unknown transport key")
	}
}

func TestParseAssistantResponseNormalizesMotion(t *testing.T) {
	speed := 35
	response, err := ParseAssistantResponse(`{"reply":"Starting.","motion":{"action":"START","pattern_id":"PULSE","speed_percent":35}}`)
	if err != nil {
		t.Fatalf("ParseAssistantResponse: %v", err)
	}
	if response.Motion == nil {
		t.Fatal("motion command missing")
	}
	if response.Motion.Action != MotionActionStart {
		t.Fatalf("action = %q, want %q", response.Motion.Action, MotionActionStart)
	}
	if response.Motion.PatternID != "pulse" {
		t.Fatalf("pattern = %q, want pulse", response.Motion.PatternID)
	}
	if response.Motion.SpeedPercent == nil || *response.Motion.SpeedPercent != speed {
		t.Fatalf("speed = %v, want %d", response.Motion.SpeedPercent, speed)
	}
}

func TestCuratedParserAcceptsOnlyEnabledPatternIDs(t *testing.T) {
	choices := []PatternChoice{{ID: "pattern-user-wave", Name: "Wave", Weight: 1.2}}
	response, err := ParseAssistantResponseWithPatterns(
		`{"reply":"Using it.","motion":{"action":"start","pattern_id":"pattern-user-wave","intensity":32}}`,
		choices,
	)
	if err != nil {
		t.Fatalf("enabled selection: %v", err)
	}
	if response.Motion == nil || response.Motion.Intensity == nil || *response.Motion.Intensity != 32 {
		t.Fatalf("curated response = %+v", response)
	}
	_, err = ParseAssistantResponseWithPatterns(
		`{"reply":"Wrong.","motion":{"action":"start","pattern_id":"stroke","intensity":32}}`,
		choices,
	)
	if err == nil || !strings.Contains(err.Error(), "unknown motion pattern") {
		t.Fatalf("disabled selection error = %v", err)
	}
}

func TestStopContractRejectsTargetFields(t *testing.T) {
	_, err := ParseAssistantResponseWithPatterns(
		`{"reply":"Stopping.","motion":{"action":"stop","pattern_id":"stroke","intensity":20}}`,
		[]PatternChoice{{ID: "stroke"}},
	)
	if err == nil {
		t.Fatal("stop contract accepted unused target fields")
	}
}

func TestNoEnabledPatternsExposeDeterministicFallback(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	prompt := ComposeSystemWithPatterns(set, nil, nil)
	if !strings.Contains(prompt, "No motion patterns are enabled") || !strings.Contains(prompt, "speed_percent") {
		t.Fatalf("fallback prompt missing:\n%s", prompt)
	}
	response, err := ParseAssistantResponseWithPatterns(
		`{"reply":"Fallback.","motion":{"action":"start","speed_percent":25}}`, nil,
	)
	if err != nil || response.Motion.SpeedPercent == nil {
		t.Fatalf("deterministic fallback = %+v err=%v", response, err)
	}
}

func TestPromptExamplesMatchStrictParserAndGuardIsLast(t *testing.T) {
	examples := []string{
		`{"reply":"short user-facing reply"}`,
		`{"reply":"short user-facing reply","motion":{"action":"none"}}`,
		`{"reply":"short user-facing reply","motion":{"action":"start","speed_percent":25}}`,
		`{"reply":"short user-facing reply","motion":{"action":"target","speed_percent":25}}`,
		`{"reply":"short user-facing reply","motion":{"action":"stop"}}`,
	}
	for _, example := range examples {
		if _, err := ParseAssistantResponseWithPatterns(example, nil); err != nil {
			t.Fatalf("prompt example %s: %v", example, err)
		}
	}
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	prompt := ComposeSystemWithPatterns(set, []string{"keep replies brief"}, []PatternChoice{{ID: "pulse", Name: "Pulse"}})
	if !strings.HasSuffix(prompt, finalOutputGuard) {
		t.Fatalf("final output guard is not last:\n%s", prompt)
	}
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.HasPrefix(line, "Valid curated ") {
			continue
		}
		_, example, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("curated example marker malformed: %q", line)
		}
		if _, err := ParseAssistantResponseWithPatterns(example, []PatternChoice{{ID: "pulse"}}); err != nil {
			t.Fatalf("curated prompt example %s: %v", example, err)
		}
	}
}

func TestServiceRepairsMalformedResponseOnce(t *testing.T) {
	provider := &scriptedProvider{responses: []string{
		"not json",
		`{"reply":"Fixed.","motion":{"action":"none"}}`,
	}}
	service := Service{
		Provider:      provider,
		Model:         "local-model",
		MaxTokens:     256,
		ReasoningMode: "off",
	}
	var events []StreamEvent

	result, err := service.Complete(t.Context(), Request{Message: "hello"}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result.Malformed {
		t.Fatalf("result remained malformed: %+v", result)
	}
	if !result.InitialMalformed || !result.Repaired {
		t.Fatalf("result repair flags = %+v, want initial malformed and repaired", result)
	}
	if result.Response.Reply != "Fixed." {
		t.Fatalf("reply = %q, want Fixed.", result.Response.Reply)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.requests))
	}
	for index, request := range provider.requests {
		if request.MaxTokens != 256 || request.ReasoningMode != "off" {
			t.Fatalf("request %d generation controls = %+v", index, request)
		}
	}
	if provider.requests[1].Temperature != 0 {
		t.Fatalf("repair temperature = %v, want 0", provider.requests[1].Temperature)
	}
	repairPrompt := provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content
	if !strings.Contains(repairPrompt, "Validation error") || strings.Contains(repairPrompt, "not json") {
		t.Fatalf("repair prompt did not include failure context: %q", repairPrompt)
	}
	if got := provider.requests[1].Messages[len(provider.requests[1].Messages)-2]; got.Role != "assistant" || got.Content != "not json" {
		t.Fatalf("malformed assistant output not retained as repair context: %+v", got)
	}
	if !sawEvent(events, "malformed") || !sawEvent(events, "repair_delta") {
		t.Fatalf("events = %+v, want malformed and repair_delta", events)
	}
}

func TestServiceReservesAutomaticReasoningAndRepairsTruncationWithoutThinking(t *testing.T) {
	provider := &scriptedProvider{
		responses:      []string{"", `{"reply":"Recovered."}`},
		responseErrors: []error{llm.ErrOutputTruncated, nil},
	}
	service := Service{
		Provider:              provider,
		Model:                 "small-local-model",
		MaxTokens:             256,
		ReasoningMode:         "auto",
		ReasoningBudgetTokens: 128,
	}
	result, err := service.Complete(t.Context(), Request{
		Message: "hello",
		History: []llm.Message{{Role: "assistant", Content: "Earlier reply."}},
	}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !result.Repaired || result.Response.Reply != "Recovered." {
		t.Fatalf("result = %+v", result)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.requests))
	}
	if initial := provider.requests[0]; initial.MaxTokens != 256 || initial.ReasoningMode != "auto" || initial.ReasoningBudgetTokens != 128 {
		t.Fatalf("initial generation controls = %+v", initial)
	}
	if repair := provider.requests[1]; repair.MaxTokens != 256 || repair.ReasoningMode != "off" || repair.ReasoningBudgetTokens != 0 {
		t.Fatalf("repair generation controls = %+v", repair)
	}
	repairMessages := provider.requests[1].Messages
	if len(repairMessages) < 5 || repairMessages[len(repairMessages)-3].Content != "hello" ||
		repairMessages[len(repairMessages)-2].Role != "assistant" ||
		repairMessages[len(repairMessages)-2].Content != emptyRepairContext ||
		strings.Contains(repairMessages[len(repairMessages)-2].Content, "motion") ||
		repairMessages[len(repairMessages)-1].Role != "user" {
		t.Fatalf("repair did not retain original conversation: %+v", repairMessages)
	}
}

func TestServiceAcceptsValidRepairAtTokenLimit(t *testing.T) {
	provider := &scriptedProvider{
		responses:      []string{"not json", `{"reply":"Complete."}`},
		responseErrors: []error{nil, llm.ErrOutputTruncated},
	}
	result, err := (Service{Provider: provider, MaxTokens: 128}).Complete(t.Context(), Request{Message: "hello"}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !result.Repaired || result.Malformed || result.Response.Reply != "Complete." {
		t.Fatalf("valid repair at token limit was rejected: %+v", result)
	}
}

func TestServiceRewritesPlainAssistantHistoryAsContractJSON(t *testing.T) {
	provider := &scriptedProvider{responses: []string{
		`{"reply":"Still here.","motion":{"action":"none"}}`,
	}}
	service := Service{
		Provider: provider,
		Model:    "local-model",
	}

	_, err := service.Complete(t.Context(), Request{
		Message: "continue",
		History: []llm.Message{
			{Role: "assistant", Content: "Plain displayed reply."},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.requests))
	}
	history := provider.requests[0].Messages[1]
	if history.Role != "assistant" {
		t.Fatalf("history role = %q, want assistant", history.Role)
	}
	if !strings.Contains(history.Content, `"reply":"Plain displayed reply."`) ||
		!strings.Contains(history.Content, `"action":"none"`) {
		t.Fatalf("assistant history was not rewritten as contract JSON: %q", history.Content)
	}
}

func sawEvent(events []StreamEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
