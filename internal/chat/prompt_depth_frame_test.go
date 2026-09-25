package chat

import (
	"strings"
	"testing"
)

// Every mode that can move the device reads positions in one depth frame, so
// no contract can again leave "deeper" free to point at the tip.
func TestDepthFrameGroundsEveryMotionMode(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	for _, mode := range []MotionMode{MotionModePattern, MotionModeDynamic, MotionModeLayered, MotionModeCreativeV2} {
		capabilities := FullCapabilities()
		capabilities.MotionMode, capabilities.Voice = mode, VoiceExplicit
		composition := ComposePrompt(set, nil, defaultPatternChoices(), capabilities,
			&MotionContext{Running: true, MotionMode: mode}, &ConversationContext{})
		index := map[string]int{}
		for i, section := range composition.Sections {
			index[section.ID] = i
		}
		frame, ok := index["depth_frame"]
		if !ok || strings.Count(composition.Prompt, depthFrame) != 1 {
			t.Fatalf("%s prompt does not state the depth frame exactly once", mode)
		}
		if frame != index["response_contract"]+1 {
			t.Fatalf("%s depth frame is not beside the contract it grounds", mode)
		}
	}
	chatOnly := ComposePrompt(set, nil, nil, Capabilities{Voice: VoiceExplicit}, nil, &ConversationContext{})
	if strings.Contains(chatOnly.Prompt, depthFrame) {
		t.Fatal("chat without motion capability composed the depth frame")
	}
}

func TestAutopilotPlanningAndSpeechShareTheDepthFrame(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	for _, kind := range []AutopilotKind{AutopilotKindMotion, AutopilotKindSpeech} {
		system := composeAutopilotSystem(set, nil, defaultPatternChoices(), FullCapabilities(),
			&MotionContext{Running: true}, nil, kind)
		if !strings.Contains(system, depthFrame) {
			t.Fatalf("%s Autopilot prompt omits the depth frame", kind)
		}
	}
	var facts strings.Builder
	writeAutopilotMotionFacts(&facts, AutopilotContext{}, false)
	if !strings.Contains(facts.String(), "0% at the base, the deepest point, and 100% at the tip, the shallowest") {
		t.Fatalf("motion facts do not say which end is deep: %q", facts.String())
	}
}
