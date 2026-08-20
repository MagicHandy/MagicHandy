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
		CenterPercent: 50, SpanPercent: 78, SpanMinPercent: 34, SpanProfile: DynamicSpanProfileWander,
		VariationPercent: 20, SegmentSeconds: 18,
		SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	system := ComposeSystemWithMotionContext(set, nil, []PatternChoice{{ID: "pulse", Name: "Pulse"}}, capabilities, context)
	for _, required := range []string{
		`"action":"update"`, "center_percent", "span_percent", "span_min_percent",
		"span_profile", "breathe", "wander", "contrast", "anchors", "segment_seconds",
		"never copy an example speed", `"motion" must be a nested JSON object`,
	} {
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

func TestMotionContextEndsWithCurrentStateActionSet(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	dynamic := Capabilities{Motion: true, MotionMode: MotionModeDynamic}
	pattern := FullCapabilities()
	tests := []struct {
		name         string
		capabilities Capabilities
		context      MotionContext
		want         string
	}{
		{"Dynamic stopped", dynamic, MotionContext{}, "state is stopped: choose start or none; update is invalid"},
		{"Dynamic running", dynamic, MotionContext{Running: true}, "state is running: choose update or none; start is invalid"},
		{"Dynamic paused", dynamic, MotionContext{Paused: true}, "state is paused: choose none; start and update are invalid"},
		{"pattern stopped", pattern, MotionContext{}, "state is stopped: choose start or none; target is invalid"},
		{"pattern running", pattern, MotionContext{Running: true}, "state is running: choose target or none; start is invalid"},
		{"pattern paused", pattern, MotionContext{Paused: true}, "state is paused: choose none; start and target are invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.context.SpeedMinPercent = 20
			test.context.SpeedMaxPercent = 40
			prompt := ComposeSystemWithMotionContext(set, nil, nil, test.capabilities, test.context)
			if !strings.Contains(prompt, test.want) {
				t.Fatalf("state-specific action set missing %q:\n%s", test.want, prompt)
			}
		})
	}
}

func TestDynamicParserAcceptsDirectGeometryAndNormalizesAnchorRoutes(t *testing.T) {
	capabilities := Capabilities{Motion: true, MotionMode: MotionModeDynamic}
	context := MotionContext{SpeedMinPercent: 20, SpeedMaxPercent: 40}
	response, err := parseAssistantResponseForCapabilities(
		`{"reply":"Starting.","motion":{"action":"start","speed_percent":30,"anchors":["tip","middle","base"],"span_min_percent":30,"span_profile":"wander","variation_percent":25,"segment_seconds":15}}`,
		nil, capabilities, &context,
	)
	if err != nil || response.Motion == nil || len(response.Motion.Anchors) != 3 ||
		response.Motion.SpanMinPercent == nil || *response.Motion.SpanMinPercent != 30 ||
		response.Motion.SpanProfile != DynamicSpanProfileWander {
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
		`{"reply":"Bad.","motion":{"action":"start","speed_percent":30,"center_percent":50,"span_percent":70,"span_profile":"wander"}}`,
		`{"reply":"Bad.","motion":{"action":"start","speed_percent":30,"center_percent":50,"span_percent":70,"span_min_percent":10,"span_profile":"wander"}}`,
		`{"reply":"Bad.","motion":{"action":"start","speed_percent":30,"center_percent":50,"span_percent":70,"span_min_percent":80,"span_profile":"wander"}}`,
		`{"reply":"Bad.","motion":{"action":"start","speed_percent":30,"anchors":["base","middle","tip"],"span_min_percent":90,"span_profile":"contrast"}}`,
		`{"reply":"Bad.","motion":{"action":"start","speed_percent":30,"center_percent":50,"span_percent":70,"span_min_percent":30,"span_profile":"random"}}`,
	} {
		if _, err := parseAssistantResponseForCapabilities(raw, []PatternChoice{{ID: "pulse"}}, capabilities, &context); err == nil {
			t.Fatalf("invalid dynamic response accepted: %s", raw)
		}
	}
}

func TestDynamicSpanEnvelopeParserAcceptsFixedAndVariableUpdates(t *testing.T) {
	capabilities := Capabilities{Motion: true, MotionMode: MotionModeDynamic}
	context := MotionContext{
		Running: true, MotionMode: MotionModeDynamic, SpeedPercent: 30,
		CenterPercent: 50, SpanPercent: 78, SpanMinPercent: 34,
		SpanProfile:     DynamicSpanProfileWander,
		SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	for _, raw := range []string{
		`{"reply":"I will let the range breathe.","motion":{"action":"update","span_min_percent":28,"span_profile":"breathe"}}`,
		`{"reply":"I will mix tight and broad strokes.","motion":{"action":"update","span_percent":82,"span_min_percent":25,"span_profile":"contrast"}}`,
		`{"reply":"I will keep the stroke length steady.","motion":{"action":"update","span_profile":"steady"}}`,
		`{"reply":"A little faster.","motion":{"action":"update","speed_percent":34}}`,
	} {
		response, err := parseAssistantResponseForCapabilities(raw, nil, capabilities, &context)
		if err != nil || response.Motion == nil || response.Motion.Action != MotionActionUpdate {
			t.Fatalf("valid span envelope update rejected: raw=%s response=%+v err=%v", raw, response, err)
		}
	}
	contrastOnly, err := parseAssistantResponseForCapabilities(
		`{"reply":"I will change the grouping.","motion":{"action":"update","span_profile":"contrast"}}`,
		nil, capabilities, &context,
	)
	if err != nil || contrastOnly.Motion == nil || contrastOnly.Motion.SpanProfile != DynamicSpanProfileContrast {
		t.Fatalf("profile-only update did not preserve the running floor: response=%+v err=%v", contrastOnly, err)
	}

	legacyContext := context
	legacyContext.SpanMinPercent = 0
	legacyContext.SpanProfile = ""
	for _, raw := range []string{
		`{"reply":"I will wander.","motion":{"action":"update","span_profile":"wander"}}`,
		`{"reply":"I will make it narrow.","motion":{"action":"update","span_percent":30}}`,
		`{"reply":"Starting.","motion":{"action":"start","speed_percent":30,"center_percent":50,"span_percent":70,"span_min_percent":70,"span_profile":"breathe"}}`,
	} {
		parseContext := &context
		if strings.Contains(raw, `"span_profile":"wander"`) {
			parseContext = &legacyContext
		}
		if _, err := parseAssistantResponseForCapabilities(raw, nil, capabilities, parseContext); err == nil {
			t.Fatalf("unusable effective span envelope accepted: %s", raw)
		}
	}
}

func TestDynamicMotionContextIncludesEffectiveSpanEnvelope(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	context := MotionContext{
		Running: true, MotionMode: MotionModeDynamic, SpeedPercent: 30,
		CenterPercent: 50, SpanPercent: 82, SpanMinPercent: 28,
		SpanProfile: DynamicSpanProfileContrast, VariationPercent: 24,
		SegmentSeconds: 18, SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	system := ComposeSystemWithMotionContext(
		set, nil, nil, Capabilities{Motion: true, MotionMode: MotionModeDynamic}, context,
	)
	for _, want := range []string{
		`"span_percent":82`, `"span_min_percent":28`, `"span_profile":"contrast"`,
		"span_percent or the anchor route defines the widest reach",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("dynamic motion context missing %q:\n%s", want, system)
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

func TestDynamicUpdateAuthorizationScopesNegativeQualifiersToTheirAxis(t *testing.T) {
	context := MotionContext{
		Running: true, MotionMode: MotionModeDynamic, SpeedPercent: 24,
		CenterPercent: 50, SpanPercent: 76, SpanMinPercent: 34,
		SpanProfile: DynamicSpanProfileWander,
	}
	spanMin := 20
	tests := []struct {
		name    string
		message string
		command MotionCommand
		want    bool
	}{
		{
			name:    "range request preserves pace",
			message: "Mix short and deep strokes smoothly without changing the pace.",
			command: MotionCommand{Action: MotionActionUpdate, SpanMinPercent: &spanMin, SpanProfile: DynamicSpanProfileWander},
			want:    true,
		},
		{
			name:    "steady range preserves pace",
			message: "Keep the stroke length perfectly steady now. Do not change the pace.",
			command: MotionCommand{Action: MotionActionUpdate, SpanProfile: DynamicSpanProfileSteady},
			want:    true,
		},
		{
			name:    "speed mutation violates qualifier",
			message: "Mix short and deep strokes without changing the pace.",
			command: MotionCommand{Action: MotionActionUpdate, SpeedPercent: intPointer(30), SpanMinPercent: &spanMin, SpanProfile: DynamicSpanProfileWander},
			want:    false,
		},
		{
			name:    "unscoped refusal",
			message: "Do not move yet; just talk to me.",
			command: MotionCommand{Action: MotionActionUpdate, SpanMinPercent: &spanMin, SpanProfile: DynamicSpanProfileWander},
			want:    false,
		},
		{
			name:    "whole-motion refusal beats a contradictory target",
			message: "Do not move yet, even if a different range might fit.",
			command: MotionCommand{Action: MotionActionUpdate, SpanMinPercent: &spanMin, SpanProfile: DynamicSpanProfileWander},
			want:    false,
		},
		{
			name:    "clear variation request",
			message: "Do not vary the range anymore.",
			command: MotionCommand{Action: MotionActionUpdate, SpanProfile: DynamicSpanProfileSteady},
			want:    true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := userAuthorizesDynamicUpdate(test.message, test.command, context); got != test.want {
				t.Fatalf("authorization = %t, want %t", got, test.want)
			}
		})
	}
}

func TestDynamicSpanAdverbDoesNotImplyDeviceSpeedBand(t *testing.T) {
	context := MotionContext{Running: true, SpeedPercent: 24, SpeedMinPercent: 10, SpeedMaxPercent: 40}
	if label, band, ok := requestedSpeedBand(context, "Let the stroke length slowly breathe, but keep exactly the same pace."); ok {
		t.Fatalf("span-texture adverb selected %s speed band %v", label, band)
	}
	if label, _, ok := requestedSpeedBand(context, "Keep moving slowly."); !ok || label != "low" {
		t.Fatalf("ordinary slowly selected label=%q ok=%t, want low", label, ok)
	}
}

func intPointer(value int) *int {
	return &value
}
