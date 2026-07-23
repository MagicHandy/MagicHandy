package chat

import (
	"strings"
	"testing"
)

func TestUtilityVoiceLeavesThePromptUnchanged(t *testing.T) {
	prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	patterns := []PatternChoice{{ID: "stroke", Name: "Stroke"}}

	capabilities := FullCapabilities()
	baseline := ComposeSystemWithCapabilities(prompt, nil, patterns, capabilities)
	capabilities.Voice = VoiceUtility
	utility := ComposeSystemWithCapabilities(prompt, nil, patterns, capabilities)

	if baseline != utility {
		t.Fatal("utility voice must compose byte-identical to the historical prompt")
	}
	if strings.Contains(utility, "CHAT VOICE") {
		t.Fatal("utility voice must not add a voice section")
	}
}

func TestVoiceLevelsComposeTheirRegisterSections(t *testing.T) {
	prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	patterns := []PatternChoice{{ID: "stroke", Name: "Stroke"}}

	for _, testCase := range []struct {
		voice    VoiceLevel
		header   string
		required []string
		banned   []string
	}{
		{
			voice:    VoiceWarm,
			header:   "CHAT VOICE - WARM COMPANION:",
			required: []string{"never explicit", "never copy or imitate their reply wording"},
			banned:   []string{"erotic"},
		},
		{
			voice:    VoiceIntimate,
			header:   "CHAT VOICE - INTIMATE PARTNER:",
			required: []string{"intimate partner in the room", "not an assistant"},
			banned:   []string{"erotic"},
		},
		{
			voice:  VoiceExplicit,
			header: "CHAT VOICE - EXPLICIT PARTNER:",
			required: []string{
				"adult erotic partner",
				"do not sanitize, euphemize",
				"never copy or imitate their reply wording",
			},
		},
	} {
		capabilities := FullCapabilities()
		capabilities.Voice = testCase.voice
		system := ComposeSystemWithCapabilities(prompt, nil, patterns, capabilities)
		if !strings.Contains(system, testCase.header) {
			t.Fatalf("%s prompt missing header %q", testCase.voice, testCase.header)
		}
		for _, want := range testCase.required {
			if !strings.Contains(system, want) {
				t.Fatalf("%s prompt missing %q", testCase.voice, want)
			}
		}
		for _, unwanted := range testCase.banned {
			if strings.Contains(system, unwanted) {
				t.Fatalf("%s prompt must not contain %q", testCase.voice, unwanted)
			}
		}
		// The voice changes register only: the machine contract and the final
		// output guard survive at every level.
		if !strings.Contains(system, `"action":"start"`) {
			t.Fatalf("%s prompt lost the motion contract", testCase.voice)
		}
		if !strings.HasSuffix(system, finalOutputGuard) {
			t.Fatalf("%s prompt displaced the final output guard", testCase.voice)
		}
	}
}

func TestVoiceSectionPrecedesMemoriesAndGuard(t *testing.T) {
	prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	capabilities := FullCapabilities()
	capabilities.Voice = VoiceIntimate
	system := ComposeSystemWithCapabilities(prompt, []string{"prefers a slow start"}, nil, capabilities)

	voiceAt := strings.Index(system, "CHAT VOICE - INTIMATE PARTNER:")
	memoriesAt := strings.Index(system, "Saved user memories")
	if voiceAt == -1 || memoriesAt == -1 || voiceAt > memoriesAt {
		t.Fatalf("voice section must precede memories: voice=%d memories=%d", voiceAt, memoriesAt)
	}
}
