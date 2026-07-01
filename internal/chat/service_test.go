package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type scriptedProvider struct {
	responses []string
	requests  []llm.ChatRequest
}

func (p *scriptedProvider) StreamChat(_ context.Context, request llm.ChatRequest, onDelta func(string) error) (string, error) {
	p.requests = append(p.requests, request)
	if len(p.responses) == 0 {
		return "", errors.New("scripted response missing")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	if onDelta != nil {
		if err := onDelta(response); err != nil {
			return response, err
		}
	}
	return response, nil
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

func TestServiceRepairsMalformedResponseOnce(t *testing.T) {
	provider := &scriptedProvider{responses: []string{
		"not json",
		`{"reply":"Fixed.","motion":{"action":"none"}}`,
	}}
	service := Service{
		Provider:    provider,
		PromptSetID: defaultPromptSetID,
		Model:       "local-model",
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
	repairPrompt := provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content
	if !strings.Contains(repairPrompt, "Validation error") || !strings.Contains(repairPrompt, "not json") {
		t.Fatalf("repair prompt did not include failure context: %q", repairPrompt)
	}
	if !sawEvent(events, "malformed") || !sawEvent(events, "repair_delta") {
		t.Fatalf("events = %+v, want malformed and repair_delta", events)
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
