package chat

import (
	"context"
	"strings"
	"testing"
)

func TestContractAcceptsNamedAreaZones(t *testing.T) {
	response, err := ParseAssistantResponse(`{"reply":"ok","motion":{"action":"target","area":"tip","speed_percent":30}}`)
	if err != nil {
		t.Fatalf("tip zone rejected: %v", err)
	}
	if response.Motion.Area != AreaZoneTip {
		t.Fatalf("area = %q, want tip", response.Motion.Area)
	}

	if _, err := ParseAssistantResponse(`{"reply":"ok","motion":{"action":"target","area":"everywhere","speed_percent":30}}`); err == nil {
		t.Fatal("unknown zone must be rejected")
	}
	if _, err := ParseAssistantResponse(`{"reply":"ok","motion":{"action":"stop","area":"tip"}}`); err == nil {
		t.Fatal("stop with area must be rejected")
	}
	if _, err := ParseAssistantResponse(`{"reply":"ok","motion":{"action":"start","area":"full"}}`); err != nil {
		t.Fatalf("start with full-area clear rejected: %v", err)
	}
}

func TestComposeSystemAdvertisesOnlyEnabledCapabilities(t *testing.T) {
	prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	patterns := []PatternChoice{{ID: "stroke", Name: "Stroke"}}

	full := ComposeSystemWithCapabilities(prompt, nil, patterns, FullCapabilities())
	if !strings.Contains(full, `"area":"tip"`) || !strings.Contains(full, "Enabled motion pattern catalog") {
		t.Fatal("full capabilities must advertise area zones and the catalog")
	}

	noArea := ComposeSystemWithCapabilities(prompt, nil, patterns, Capabilities{Motion: true, Patterns: true})
	if strings.Contains(noArea, `"area"`) {
		t.Fatal("disabled area focus must not be described to the model")
	}

	noPatterns := ComposeSystemWithCapabilities(prompt, nil, patterns, Capabilities{Motion: true, AreaFocus: true})
	if strings.Contains(noPatterns, "Enabled motion pattern catalog") {
		t.Fatal("disabled patterns must hide the catalog")
	}
	if !strings.Contains(noPatterns, "No motion patterns are enabled") {
		t.Fatal("pattern-less contract must state the deterministic fallback")
	}

	chatOnly := ComposeSystemWithCapabilities(prompt, nil, patterns, Capabilities{})
	if strings.Contains(chatOnly, `"action":"start"`) || strings.Contains(chatOnly, "catalog") {
		t.Fatal("chat-only contract must not describe motion shapes")
	}
	if !strings.Contains(chatOnly, "Motion control is disabled") {
		t.Fatal("chat-only contract must explain the disabled state")
	}
}

func TestCompleteEnforcesCapabilitiesWithoutFailingTheTurn(t *testing.T) {
	patterns := []PatternChoice{{ID: "stroke", Name: "Stroke"}}

	// Chat-only: an emitted motion command is stripped, never an error.
	chatOnly := Capabilities{}
	service := Service{
		Provider:     &scriptedProvider{responses: []string{`{"reply":"ok","motion":{"action":"start","speed_percent":30}}`}},
		Patterns:     patterns,
		Capabilities: &chatOnly,
	}
	result, err := service.Complete(context.Background(), Request{Message: "go"}, nil)
	if err != nil || result.Malformed {
		t.Fatalf("chat-only enforcement failed the turn: %v %+v", err, result)
	}
	if result.Response.Motion != nil {
		t.Fatalf("chat-only must strip motion, got %+v", result.Response.Motion)
	}

	// Area disabled: the zone is dropped, the rest of the command survives.
	noArea := Capabilities{Motion: true, Patterns: true}
	service = Service{
		Provider:     &scriptedProvider{responses: []string{`{"reply":"ok","motion":{"action":"target","area":"tip","speed_percent":40}}`}},
		Patterns:     patterns,
		Capabilities: &noArea,
	}
	result, err = service.Complete(context.Background(), Request{Message: "tip"}, nil)
	if err != nil || result.Response.Motion == nil {
		t.Fatalf("area enforcement broke the command: %v %+v", err, result)
	}
	if result.Response.Motion.Area != "" || *result.Response.Motion.SpeedPercent != 40 {
		t.Fatalf("area must be stripped and speed kept: %+v", result.Response.Motion)
	}

	// Patterns disabled: the curated choice degrades to the deterministic
	// speed fallback instead of erroring.
	noPatterns := Capabilities{Motion: true, AreaFocus: true}
	service = Service{
		Provider:     &scriptedProvider{responses: []string{`{"reply":"ok","motion":{"action":"target","pattern_id":"stroke","intensity":35}}`}},
		Patterns:     patterns,
		Capabilities: &noPatterns,
	}
	result, err = service.Complete(context.Background(), Request{Message: "pattern"}, nil)
	if err != nil || result.Response.Motion == nil {
		t.Fatalf("pattern enforcement broke the command: %v %+v", err, result)
	}
	motion := result.Response.Motion
	if motion.PatternID != "" || motion.Intensity != nil || motion.SpeedPercent == nil || *motion.SpeedPercent != 35 {
		t.Fatalf("curated choice must degrade to speed fallback: %+v", motion)
	}
}
