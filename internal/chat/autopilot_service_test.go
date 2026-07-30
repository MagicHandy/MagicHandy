package chat

import (
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
		`{"motion":{"action":"target","pattern_id":"pulse","intensity":35},"next":"later"}`,
		AutopilotKindMotion,
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if response.Reply != "" || response.Next != AutopilotTimingLater ||
		response.Motion == nil || response.Motion.PatternID != "pulse" {
		t.Fatalf("response = %+v", response)
	}
	if _, err := service.parse(
		`{"reply":"coupled speech","motion":{"action":"none"},"next":"normal"}`,
		AutopilotKindMotion,
	); err == nil {
		t.Fatal("motion contract accepted a reply field")
	}
}

func TestAutopilotContractRejectsStartAndNumericTiming(t *testing.T) {
	service := AutopilotService{Capabilities: FullCapabilities(), Patterns: defaultPatternChoices()}
	if _, err := service.parse(
		`{"motion":{"action":"start","speed_percent":30},"next":"soon"}`,
		AutopilotKindMotion,
	); err == nil || !strings.Contains(err.Error(), "target or none") {
		t.Fatalf("start error = %v", err)
	}
	if _, err := service.parse(
		`{"motion":{"action":"none"},"next":30}`,
		AutopilotKindMotion,
	); err == nil {
		t.Fatal("motion contract accepted numeric timing")
	}
}

func TestAutopilotSpeechAuthorityStripsUnadvertisedMotion(t *testing.T) {
	service := AutopilotService{Capabilities: Capabilities{Voice: VoiceWarm}}
	response, err := service.parse(
		`{"reply":"Still here with you.","motion":{"action":"target","speed_percent":40},"next":"normal"}`,
		AutopilotKindSpeech,
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if response.Reply == "" || response.Motion != nil || response.Next != AutopilotTimingNormal {
		t.Fatalf("chat-only speech response = %+v", response)
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
		`Do not include a "reply" field`,
		`"next":"soon"|"normal"|"later"`,
		"No reply text",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("motion prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `Every response requires a non-empty "reply"`) {
		t.Fatal("interactive reply contract leaked into motion prompt")
	}
}
