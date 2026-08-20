package chat

import (
	"strings"
	"testing"
)

func TestDynamicContractExcludesPatternVocabulary(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	capabilities := Capabilities{Motion: true, MotionMode: MotionModeDynamic}
	context := MotionContext{
		Running: true, MotionMode: MotionModeDynamic, SpeedPercent: 30,
		CenterPercent: 50, SpanPercent: 70, VariationPercent: 20, SegmentSeconds: 12,
		SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	system := ComposeSystemWithMotionContext(set, nil, []PatternChoice{{ID: "pulse", Name: "Pulse"}}, capabilities, context)
	for _, required := range []string{`"action":"update"`, "center_percent", "span_percent", "anchors", "segment_seconds"} {
		if !strings.Contains(system, required) {
			t.Fatalf("dynamic prompt missing %q", required)
		}
	}
	for _, forbidden := range []string{"Enabled motion pattern catalog", "pattern_id may", `"area":"tip"`} {
		if strings.Contains(system, forbidden) {
			t.Fatalf("dynamic prompt contains pattern vocabulary %q", forbidden)
		}
	}
}

func TestDynamicPromptKeepsVolatileMotionContextNearEnd(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	capabilities := Capabilities{
		Motion: true, MotionMode: MotionModeDynamic, Voice: VoiceIntimate, MoodTracking: true,
	}
	conversation := ConversationContext{PersonaName: "Morgan", PersonaDescription: "Reserved and direct."}
	context := MotionContext{
		Running: true, MotionMode: MotionModeDynamic, SpeedPercent: 30,
		CenterPercent: 50, SpanPercent: 70, SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	composition := ComposePrompt(set, nil, nil, capabilities, &context, &conversation)
	indices := make(map[string]int, len(composition.Sections))
	for index, section := range composition.Sections {
		indices[section.ID] = index
	}
	for _, stable := range []string{"response_contract", "conversation_context", "voice_check"} {
		if indices[stable] >= indices["motion_context"] {
			t.Fatalf("%s section must precede volatile motion context: %#v", stable, indices)
		}
	}
	if indices["motion_context"]+1 != indices["output_guard"] {
		t.Fatalf("motion context must immediately precede final output guard: %#v", indices)
	}
	if indices["output_guard"] != len(composition.Sections)-1 {
		t.Fatalf("output guard must remain final: %#v", indices)
	}
}

func TestDynamicParserAcceptsDirectGeometryAndNormalizesAnchorRoutes(t *testing.T) {
	capabilities := Capabilities{Motion: true, MotionMode: MotionModeDynamic}
	context := MotionContext{SpeedMinPercent: 20, SpeedMaxPercent: 40}
	response, err := parseAssistantResponseForCapabilities(
		`{"reply":"Starting.","motion":{"action":"start","speed_percent":30,"anchors":["tip","middle","base"],"variation_percent":25,"segment_seconds":15}}`,
		nil, capabilities, &context,
	)
	if err != nil || response.Motion == nil || len(response.Motion.Anchors) != 3 {
		t.Fatalf("dynamic anchor start = %+v, %v", response, err)
	}
	mixed, err := parseAssistantResponseForCapabilities(
		`{"reply":"Starting.","motion":{"action":"start","speed_percent":30,"anchors":["base","middle","tip"],"center_percent":50,"span_percent":70}}`,
		nil, capabilities, &context,
	)
	if err != nil || mixed.Motion == nil || len(mixed.Motion.Anchors) != 3 ||
		mixed.Motion.CenterPercent != nil || mixed.Motion.SpanPercent != nil {
		t.Fatalf("mixed anchor route was not normalized = %+v, %v", mixed, err)
	}
	for _, raw := range []string{
		`{"reply":"Bad.","motion":{"action":"start","speed_percent":30}}`,
		`{"reply":"Bad.","motion":{"action":"update","pattern_id":"pulse"}}`,
		`{"reply":"Bad.","motion":{"action":"update","span_percent":10}}`,
	} {
		if _, err := parseAssistantResponseForCapabilities(raw, []PatternChoice{{ID: "pulse"}}, capabilities, &context); err == nil {
			t.Fatalf("invalid dynamic response accepted: %s", raw)
		}
	}
}

func TestDynamicModeKeepsValidNoneAndUpdateDecisionsModelOwned(t *testing.T) {
	capabilities := Capabilities{Motion: true, MotionMode: MotionModeDynamic}
	context := MotionContext{
		Running: true, MotionMode: MotionModeDynamic, SpeedPercent: 30,
		CenterPercent: 50, SpanPercent: 70, VariationPercent: 15, SegmentSeconds: 12,
		SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	holdProvider := &scriptedProvider{responses: []string{`{"reply":"Holding it.","motion":{"action":"none"}}`}}
	service := Service{Provider: holdProvider, MotionContext: &context, Capabilities: &capabilities}
	result, err := service.Complete(t.Context(), Request{Message: "Keep changing it up"}, nil)
	if err != nil || result.Repaired || result.Response.Motion == nil || result.Response.Motion.Action != MotionActionNone {
		t.Fatalf("valid dynamic none decision was replaced: result=%+v err=%v", result, err)
	}
	if len(holdProvider.requests) != 1 {
		t.Fatalf("valid none decision used %d provider calls, want 1", len(holdProvider.requests))
	}

	updateProvider := &scriptedProvider{responses: []string{`{"reply":"I am changing the feel.","motion":{"action":"update","center_percent":35,"span_percent":50}}`}}
	service.Provider = updateProvider
	updated, err := service.Complete(t.Context(), Request{Message: "Tell me what you are thinking"}, nil)
	if err != nil || updated.Response.Motion == nil || updated.Response.Motion.Action != MotionActionUpdate {
		t.Fatalf("active dynamic model update was not admitted: result=%+v err=%v", updated, err)
	}
}
