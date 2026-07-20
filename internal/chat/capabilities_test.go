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
	if strings.Contains(noPatterns, "catalog") || strings.Contains(noPatterns, "pattern_id") || strings.Contains(noPatterns, "intensity") {
		t.Fatal("disabled pattern selection must disappear from the model contract")
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
		Provider:     &scriptedProvider{responses: []string{`{"reply":"ok","motion":{"action":"invented","area":"everywhere","speed_percent":30}}`}},
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
		Provider:     &scriptedProvider{responses: []string{`{"reply":"ok","motion":{"action":"target","area":"everywhere","speed_percent":40}}`}},
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
	provider := &scriptedProvider{responses: []string{`{"reply":"ok","motion":{"action":"target","pattern_id":"invented","intensity":35}}`}}
	service = Service{
		Provider:     provider,
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
	if len(provider.requests) != 1 {
		t.Fatalf("disabled pattern noise triggered repair: provider requests = %d", len(provider.requests))
	}
}

func TestComposeSystemUsesAuthoritativeMotionContext(t *testing.T) {
	prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	context := MotionContext{
		Running: true, PatternID: "pulse", SpeedPercent: 30, Area: AreaZoneTip,
		RecentPatternIDs: []string{"old", "older", "stroke", "pulse", "waves", "tease"},
		SpeedMinPercent:  20, SpeedMaxPercent: 40,
	}
	system := ComposeSystemWithMotionContext(prompt, nil,
		[]PatternChoice{{ID: "pulse", Name: "Pulse"}, {ID: "waves", Name: "Waves"}, {ID: "sway", Name: "Sway"}},
		FullCapabilities(), context)

	for _, want := range []string{
		`"state":"running"`, `"pattern_id":"pulse"`, `"speed_percent":30`,
		`"recent_pattern_ids":["stroke","pulse","waves","tease"]`,
		`"area":"tip"`, `"low":[20,26]`, `"middle":[27,33]`, `"high":[34,40]`,
		`Fresh enabled pattern IDs (current and recent patterns excluded): ["sway"]`,
		`For "continue", "steady", "same", or "hold it there"`,
		`For an explicit request to vary, mix up, surprise, or change the feel`,
		`For a pacing-only request, keep the current pattern`,
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("motion context prompt missing %q:\n%s", want, system)
		}
	}
	if !strings.HasSuffix(system, finalOutputGuard) {
		t.Fatal("motion context displaced the final output guard")
	}
}

func TestMotionContextHidesDisabledMethods(t *testing.T) {
	prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	context := MotionContext{
		Running: true, PatternID: "pulse", SpeedPercent: 30, Area: AreaZoneTip,
		SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	system := ComposeSystemWithMotionContext(prompt, nil, nil, Capabilities{Motion: true}, context)
	if strings.Contains(system, "pattern_id") || strings.Contains(system, `"area"`) {
		t.Fatalf("disabled methods leaked into motion context:\n%s", system)
	}
	if !strings.Contains(system, `"speed_percent":30`) {
		t.Fatal("allowed speed state disappeared with other methods disabled")
	}
}

func TestRunningPatternChangePreservesCurrentSpeedWhenModelOmitsPace(t *testing.T) {
	capabilities := FullCapabilities()
	context := MotionContext{Running: true, PatternID: "sway", SpeedPercent: 30, SpeedMinPercent: 20, SpeedMaxPercent: 40}
	provider := &scriptedProvider{responses: []string{
		`{"reply":"Switching patterns.","motion":{"action":"target","pattern_id":"pulse"}}`,
	}}
	service := Service{
		Provider: provider, Patterns: []PatternChoice{{ID: "pulse"}, {ID: "sway"}},
		MotionContext: &context, Capabilities: &capabilities,
	}

	result, err := service.Complete(t.Context(), Request{Message: "Use a different pattern"}, nil)
	if err != nil || result.Malformed || result.Response.Motion == nil {
		t.Fatalf("running pattern change failed: result=%+v err=%v", result, err)
	}
	command := result.Response.Motion
	if command.PatternID != "pulse" || command.SpeedPercent == nil || *command.SpeedPercent != 30 {
		t.Fatalf("pattern change did not preserve current speed: %+v", command)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("safe speed preservation triggered repair: provider requests = %d", len(provider.requests))
	}
}

func TestNoOpTargetUsesRepairWithoutForcingSteadyTurns(t *testing.T) {
	capabilities := FullCapabilities()
	context := MotionContext{Running: true, PatternID: "waves", SpeedPercent: 30, Area: AreaZoneFull, SpeedMinPercent: 20, SpeedMaxPercent: 40}
	provider := &scriptedProvider{responses: []string{
		`{"reply":"Changing it.","motion":{"action":"target","speed_percent":30}}`,
		`{"reply":"Switching patterns.","motion":{"action":"target","pattern_id":"pulse"}}`,
	}}
	service := Service{
		Provider: provider, Patterns: []PatternChoice{{ID: "pulse"}, {ID: "waves"}},
		MotionContext: &context, Capabilities: &capabilities,
	}

	result, err := service.Complete(t.Context(), Request{Message: "Change the feel"}, nil)
	if err != nil || result.Malformed || !result.Repaired || result.Response.Motion == nil {
		t.Fatalf("no-op repair failed: result=%+v err=%v", result, err)
	}
	command := result.Response.Motion
	if command.PatternID != "pulse" || command.SpeedPercent == nil || *command.SpeedPercent != 30 {
		t.Fatalf("repaired target = %+v, want a new pattern at current speed", command)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("provider requests = %d, want one repair", len(provider.requests))
	}

	steadyProvider := &scriptedProvider{responses: []string{`{"reply":"Holding steady.","motion":{"action":"none"}}`}}
	service.Provider = steadyProvider
	steady, err := service.Complete(t.Context(), Request{Message: "Keep it steady"}, nil)
	if err != nil || steady.Repaired || steady.Response.Motion == nil || steady.Response.Motion.Action != MotionActionNone {
		t.Fatalf("steady turn was forced to vary: result=%+v err=%v", steady, err)
	}
}

func TestRepeatedSemanticFailureUsesFreshPatternAndPreservesOtherChanges(t *testing.T) {
	capabilities := FullCapabilities()
	context := MotionContext{
		Running: true, PatternID: "pulse", RecentPatternIDs: []string{"tease"},
		SpeedPercent: 30, Area: AreaZoneTip, SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	provider := &scriptedProvider{responses: []string{
		`{"reply":"Changing it.","motion":{"action":"target","speed_percent":30}}`,
		`{"reply":"Changing it.","motion":{"action":"target","pattern_id":"tease","speed_percent":35,"area":"base"}}`,
	}}
	service := Service{
		Provider:      provider,
		Patterns:      []PatternChoice{{ID: "stroke"}, {ID: "pulse"}, {ID: "tease"}, {ID: "waves"}},
		MotionContext: &context, Capabilities: &capabilities,
	}

	result, err := service.Complete(t.Context(), Request{Message: "Mix it up, focus on the base, and go a little faster"}, nil)
	if err != nil || result.Malformed || !result.Repaired || !result.SemanticFallback || result.Response.Motion == nil {
		t.Fatalf("semantic fallback failed: result=%+v err=%v", result, err)
	}
	command := result.Response.Motion
	if command.PatternID != "waves" || command.SpeedPercent == nil || *command.SpeedPercent != 35 || command.Area != AreaZoneBase {
		t.Fatalf("semantic fallback target = %+v, want fresh pattern with repaired speed and area", command)
	}
}

func TestRepeatedNoOpPreservesOrdinaryConversationWithoutMotion(t *testing.T) {
	capabilities := FullCapabilities()
	context := MotionContext{
		Running: true, PatternID: "pulse", SpeedPercent: 30, Area: AreaZoneFull,
		SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	provider := &scriptedProvider{responses: []string{
		`{"reply":"I hear you.","motion":{"action":"target","speed_percent":30}}`,
		`{"reply":"I hear you.","motion":{"action":"target","pattern_id":"pulse","speed_percent":30}}`,
	}}
	service := Service{
		Provider: provider, Patterns: []PatternChoice{{ID: "pulse"}, {ID: "waves"}},
		MotionContext: &context, Capabilities: &capabilities,
	}

	result, err := service.Complete(t.Context(), Request{Message: "How are you doing?"}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result.Malformed || !result.Repaired || !result.SemanticFallback || result.Response.Motion != nil {
		t.Fatalf("ordinary conversation was not salvaged without motion: %+v", result)
	}
}

func TestPatternVariationIntentDoesNotCaptureConversationalObjects(t *testing.T) {
	for _, message := range []string{
		"Surprise me with a joke",
		"Add some variation to your wording",
		"Give me something different to think about",
	} {
		if requestsPatternVariation(message) {
			t.Errorf("requestsPatternVariation(%q) = true, want false", message)
		}
	}
	for _, message := range []string{
		"Mix it up",
		"Surprise me again",
		"Change the feel again, but keep the same pace",
		"Mix it up, focus on the base, and go a little faster",
	} {
		if !requestsPatternVariation(message) {
			t.Errorf("requestsPatternVariation(%q) = false, want true", message)
		}
	}
}

func TestExplicitVariationRepairWithoutMotionUsesFreshFallback(t *testing.T) {
	capabilities := FullCapabilities()
	context := MotionContext{
		Running: true, PatternID: "waves", RecentPatternIDs: []string{"pulse", "waves"},
		SpeedPercent: 30, Area: AreaZoneFull, SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	provider := &scriptedProvider{responses: []string{
		`{"reply":"Changing it.","motion":{"action":"target","speed_percent":30}}`,
		`{"reply":"Changing the feel."}`,
	}}
	service := Service{
		Provider:      provider,
		Patterns:      []PatternChoice{{ID: "pulse"}, {ID: "waves"}, {ID: "climb"}},
		MotionContext: &context, Capabilities: &capabilities,
	}

	result, err := service.Complete(t.Context(), Request{Message: "Change the feel again, but keep a moderate pace"}, nil)
	if err != nil || result.Malformed || !result.SemanticFallback || result.Response.Motion == nil {
		t.Fatalf("missing-variation fallback failed: result=%+v err=%v", result, err)
	}
	command := result.Response.Motion
	if command.PatternID != "climb" || command.SpeedPercent == nil || *command.SpeedPercent != 30 {
		t.Fatalf("fallback command = %+v, want fresh climb at current moderate speed", command)
	}
}

func TestExplicitVariationRejectsRecentPatternButSteadyRequestsPreserveIt(t *testing.T) {
	capabilities := FullCapabilities()
	context := MotionContext{
		Running: true, PatternID: "stroke", RecentPatternIDs: []string{"pulse", "tease"},
		SpeedPercent: 30, Area: AreaZoneFull, SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	provider := &scriptedProvider{responses: []string{
		`{"reply":"Changing it.","motion":{"action":"target","pattern_id":"pulse","speed_percent":30}}`,
		`{"reply":"Changing it.","motion":{"action":"target","pattern_id":"waves","speed_percent":30}}`,
	}}
	service := Service{
		Provider:      provider,
		Patterns:      []PatternChoice{{ID: "stroke"}, {ID: "pulse"}, {ID: "tease"}, {ID: "waves"}},
		MotionContext: &context, Capabilities: &capabilities,
	}

	result, err := service.Complete(t.Context(), Request{Message: "Mix it up again"}, nil)
	if err != nil || result.Malformed || !result.Repaired || result.Response.Motion == nil {
		t.Fatalf("recent-pattern repair failed: result=%+v err=%v", result, err)
	}
	if result.Response.Motion.PatternID != "waves" {
		t.Fatalf("repaired pattern = %q, want fresh waves", result.Response.Motion.PatternID)
	}

	steadyProvider := &scriptedProvider{responses: []string{
		`{"reply":"Keeping the pace.","motion":{"action":"target","pattern_id":"pulse","speed_percent":32}}`,
	}}
	service.Provider = steadyProvider
	steady, err := service.Complete(t.Context(), Request{Message: "A little faster, keep the same pattern"}, nil)
	if err != nil || steady.Repaired || steady.Response.Motion == nil || steady.Response.Motion.PatternID != "pulse" {
		t.Fatalf("pacing request incorrectly rejected recent pattern: result=%+v err=%v", steady, err)
	}
}

func TestClearSpeedBandRequestsReceiveOneRepair(t *testing.T) {
	capabilities := FullCapabilities()
	context := MotionContext{SpeedMinPercent: 20, SpeedMaxPercent: 40}
	provider := &scriptedProvider{responses: []string{
		`{"reply":"Starting gently.","motion":{"action":"start","speed_percent":27}}`,
		`{"reply":"Starting gently.","motion":{"action":"start","speed_percent":25}}`,
	}}
	service := Service{Provider: provider, MotionContext: &context, Capabilities: &capabilities}

	result, err := service.Complete(t.Context(), Request{Message: "Start moving gently"}, nil)
	if err != nil || result.Malformed || !result.Repaired || result.Response.Motion == nil {
		t.Fatalf("speed-band repair failed: result=%+v err=%v", result, err)
	}
	if speed := result.Response.Motion.SpeedPercent; speed == nil || *speed != 25 {
		t.Fatalf("repaired speed = %v, want 25", speed)
	}
}

func TestSpeedBandValidationUsesEffectiveCurrentSpeed(t *testing.T) {
	context := MotionContext{Running: true, SpeedPercent: 30, SpeedMinPercent: 20, SpeedMaxPercent: 40}
	command := MotionCommand{Action: MotionActionTarget, Area: AreaZoneTip}
	if err := validateRequestedSpeedBand(command, context, "Focus there at a moderate pace"); err != nil {
		t.Fatalf("current middle-band speed rejected: %v", err)
	}
	if err := validateRequestedSpeedBand(command, context, "Focus there gently"); err == nil {
		t.Fatal("current middle-band speed accepted for a low-band request")
	}
}
