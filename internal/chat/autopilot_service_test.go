package chat

import (
	"context"
	"strings"
	"testing"
)

func TestAutopilotMotionContractRequiresTimingWithoutReply(t *testing.T) {
	service := AutopilotService{
		Capabilities: FullCapabilities(),
		Patterns:     defaultPatternChoices(),
		MotionContext: &MotionContext{
			Running: true, PatternID: "stroke", SpeedPercent: 30,
		},
	}
	response, err := service.parse(
		`{"intent":"change to a stronger pulse","motion":{"action":"target","pattern_id":"pulse","intensity":35},"next":"later","variability":"restless"}`,
		AutopilotKindMotion,
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if response.Reply != "" || response.Intent != "change to a stronger pulse" || response.Next != AutopilotTimingLater ||
		response.Variability != "restless" || response.Motion == nil || response.Motion.PatternID != "pulse" {
		t.Fatalf("response = %+v", response)
	}
	if _, err := service.parse(
		`{"intent":"hold steady","reply":"coupled speech","motion":{"action":"none"},"next":"normal","variability":"normal"}`,
		AutopilotKindMotion,
	); err == nil {
		t.Fatal("motion contract accepted a reply field")
	}
	if _, err := service.parse(
		`{"motion":{"action":"none"},"next":"normal","variability":"normal"}`,
		AutopilotKindMotion,
	); err == nil || !strings.Contains(err.Error(), "intent is required") {
		t.Fatalf("missing intent error = %v", err)
	}
}

func TestAutopilotContractRejectsStartAndNumericTiming(t *testing.T) {
	service := AutopilotService{Capabilities: FullCapabilities(), Patterns: defaultPatternChoices()}
	if _, err := service.parse(
		`{"intent":"begin a steady stroke","motion":{"action":"start","speed_percent":30},"next":"soon","variability":"normal"}`,
		AutopilotKindMotion,
	); err == nil || !strings.Contains(err.Error(), "target, update, or none") {
		t.Fatalf("start error = %v", err)
	}
	if _, err := service.parse(
		`{"intent":"hold steady","motion":{"action":"none"},"next":30,"variability":"normal"}`,
		AutopilotKindMotion,
	); err == nil {
		t.Fatal("motion contract accepted numeric timing")
	}
}

func TestAutopilotCompleteSkipsIneffectiveRepairOutsideCurrentMenu(t *testing.T) {
	provider := &scriptedProvider{responses: []string{
		`{"intent":"switch to a pulse","motion":{"action":"target","pattern_id":"pulse","intensity":35},"next":"normal","variability":"normal"}`,
	}}
	service := AutopilotService{
		Provider:     provider,
		Capabilities: FullCapabilities(),
		Patterns:     []PatternChoice{{ID: "stroke"}, {ID: "tease"}},
	}
	_, err := service.Complete(context.Background(), AutopilotKindMotion, Request{Message: "Choose the next stretch."})
	if err == nil || !strings.Contains(err.Error(), `unknown motion pattern "pulse"`) {
		t.Fatalf("Complete error = %v, want the allow-list violation", err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("provider requests = %d, want no ineffective repair", len(provider.requests))
	}
}

func TestAutopilotMotionContractRequiresModelOwnedVariability(t *testing.T) {
	service := AutopilotService{Capabilities: FullCapabilities(), Patterns: defaultPatternChoices()}
	for name, raw := range map[string]string{
		"missing": `{"intent":"hold steady","motion":{"action":"none"},"next":"later"}`,
		"unknown": `{"intent":"hold steady","motion":{"action":"none"},"next":"later","variability":"chaotic"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.parse(raw, AutopilotKindMotion); err == nil ||
				!strings.Contains(err.Error(), "variability") {
				t.Fatalf("parse error = %v, want variability validation", err)
			}
		})
	}
}

func TestAutopilotContractRejectsBuildupMutation(t *testing.T) {
	service := AutopilotService{Capabilities: FullCapabilities(), Patterns: defaultPatternChoices()}
	_, err := service.parse(
		`{"intent":"hold steady","motion":{"action":"none"},"next":"normal","variability":"settled","arc":"advance"}`,
		AutopilotKindMotion,
	)
	if err == nil || !strings.Contains(err.Error(), `unknown field "arc"`) {
		t.Fatalf("arc mutation error = %v, want strict unknown-field rejection", err)
	}
	if contract := autopilotContract(AutopilotKindMotion, FullCapabilities()); strings.Contains(contract, `"arc"`) {
		t.Fatalf("autopilot contract still advertises buildup mutation:\n%s", contract)
	}
	for _, kind := range []AutopilotKind{AutopilotKindMotion, AutopilotKindSpeech} {
		if guard := autopilotOutputGuard(kind, FullCapabilities()); strings.Contains(guard, `"arc"`) {
			t.Fatalf("%s output guard still advertises buildup mutation:\n%s", kind, guard)
		}
	}
}

func TestAutopilotSpeechAuthorityStripsUnadvertisedMotion(t *testing.T) {
	service := AutopilotService{Capabilities: Capabilities{Voice: VoiceWarm}}
	response, err := service.parse(
		`{"reply":"Still here with you.","motion":{"action":"target","speed_percent":40},"next":"normal","variability":"restless"}`,
		AutopilotKindSpeech,
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if response.Reply == "" || response.Motion != nil || response.Next != AutopilotTimingNormal || response.Variability != "" {
		t.Fatalf("chat-only speech response = %+v", response)
	}
}

func TestAutopilotSpeechTargetRequiresModelOwnedVariability(t *testing.T) {
	service := AutopilotService{
		Capabilities: FullCapabilities(),
		Patterns:     defaultPatternChoices(),
		MotionContext: &MotionContext{
			Running: true, PatternID: "stroke", SpeedPercent: 30,
		},
	}
	for name, raw := range map[string]string{
		"missing": `{"reply":"Changing it.","motion":{"action":"target","speed_percent":40},"next":"normal"}`,
		"unknown": `{"reply":"Changing it.","motion":{"action":"target","speed_percent":40},"next":"normal","variability":"chaotic"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.parse(raw, AutopilotKindSpeech); err == nil ||
				!strings.Contains(err.Error(), "variability") {
				t.Fatalf("parse error = %v, want variability validation", err)
			}
		})
	}

	response, err := service.parse(
		`{"reply":"Changing it.","motion":{"action":"target","speed_percent":40},"next":"normal","variability":"restless"}`,
		AutopilotKindSpeech,
	)
	if err != nil || response.Motion == nil || response.Variability != "restless" {
		t.Fatalf("speech target response = %+v, error = %v", response, err)
	}
}

func TestComposeAutopilotMotionPromptHasNoInteractiveReplyContract(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	prompt := composeAutopilotSystem(
		set,
		nil,
		defaultPatternChoices(),
		FullCapabilities(),
		nil,
		nil,
		AutopilotKindMotion,
	)
	for _, want := range []string{
		"AUTOPILOT MOTION AUTHORITY",
		"active Autopilot mode and saved controls are an ongoing request",
		`"intent":"<one brief motion concept>"`,
		`Do not include a "reply" field`,
		`"next":"soon"|"normal"|"later"`,
		`"variability":"settled"|"normal"|"restless"`,
		"No reply text",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("motion prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `Every response requires a non-empty "reply"`) {
		t.Fatal("interactive reply contract leaked into motion prompt")
	}
	speech := composeAutopilotSystem(
		set, nil, defaultPatternChoices(), FullCapabilities(), nil, nil, AutopilotKindSpeech,
	)
	if strings.Contains(speech, "AUTOPILOT MOTION AUTHORITY") {
		t.Fatal("motion-only autonomous authority leaked into the speech prompt")
	}
}

func TestComposeDynamicAutopilotPromptOmitsInteractiveMotionContract(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	capabilities := FullCapabilities()
	capabilities.MotionMode = MotionModeDynamic
	capabilities.Patterns = false
	capabilities.AreaFocus = false
	context := MotionContext{
		MotionMode: MotionModeDynamic, SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	prompt := composeAutopilotSystem(
		set, nil, nil, capabilities, &context, nil, AutopilotKindMotion,
	)
	for _, forbidden := range []string{
		"Authoritative current motion state",
		`If state is "stopped", use action "start"`,
		"ordinary conversation normally means",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("interactive motion instruction %q leaked into Autopilot:\n%s", forbidden, prompt)
		}
	}
	if !strings.Contains(prompt, "provide or change Dynamic motion") {
		t.Fatalf("dedicated Dynamic Autopilot contract is missing:\n%s", prompt)
	}
}

func TestComposeAutopilotSpeechPromptMatchesMotionAuthority(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	full := composeAutopilotSystem(set, nil, defaultPatternChoices(), FullCapabilities(), nil, nil, AutopilotKindSpeech)
	for _, want := range []string{`target motion requires "variability"`, `matching "variability"`} {
		if !strings.Contains(full, want) {
			t.Fatalf("full-motion speech prompt missing %q:\n%s", want, full)
		}
	}

	chatOnly := composeAutopilotSystem(set, nil, nil, Capabilities{Voice: VoiceWarm}, nil, nil, AutopilotKindSpeech)
	if !strings.Contains(chatOnly, "No motion.") || strings.Contains(chatOnly, `matching "variability"`) {
		t.Fatalf("chat-only speech output guard contradicts capabilities:\n%s", chatOnly)
	}
}
