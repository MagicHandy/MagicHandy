package chat

import (
	"strings"
	"testing"
)

// Advanced chat reads a scene's reactions as motion feedback and keeps the reply
// and the motion in step. Simple/Intermediate keep their wording, and a vibrator-only
// session that cannot play is not told to move.
func TestHostedChatKeepsSceneReplyAndMotionInStep(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	hostedOnly := []string{hostedSceneGuide, "invites you to begin, never for a reaction to earlier motion",
		`Questions and talk that is not about the motion require "edits":`}
	localOnly := []string{"A stopped device starts only for a direct motion request", "Ordinary conversation and questions require reply only.",
		`Conversation requires "edits":`}
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, level := range []string{"simple", "intermediate", "advanced"} {
			advanced := level == "advanced"
			score := DefaultLayeredScore(30)
			if mode == MotionModeCreativeV2 {
				score = FreshCreativeV2Score(30)
			}
			state := MotionContext{Running: true, MotionMode: mode, Layered: &score, SpeedMinPercent: 1, SpeedMaxPercent: 100}
			caps := Capabilities{Motion: true, MotionMode: mode, AdvancedMotion: advanced, IntermediateMotion: level == "intermediate",
				SimpleMotion: level == "simple", PreserveConversationText: advanced, Voice: VoiceIntimate, Style: StyleTeasing}
			prompt := ComposeInteractivePrompt(set, nil, nil, caps, &state, nil).Prompt
			present, absent := hostedOnly, localOnly
			if !advanced {
				present, absent = localOnly, hostedOnly
			}
			for _, text := range present {
				if !strings.Contains(prompt, text) {
					t.Errorf("%s advanced=%t lacks %q", mode, advanced, text)
				}
			}
			for _, text := range absent {
				if strings.Contains(prompt, text) {
					t.Errorf("%s advanced=%t carries %q", mode, advanced, text)
				}
			}
		}
	}
	score := DefaultLayeredScore(30)
	stopped := MotionContext{MotionMode: MotionModeLayered, Layered: &score, SpeedMinPercent: 1, SpeedMaxPercent: 100,
		Outputs: &OutputContext{Vibrator: &VibrationContext{Ready: true, Stopped: true}}}
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
	if strings.Contains(ComposeInteractivePrompt(set, nil, nil, caps, &stopped, nil).Prompt, hostedSceneGuide) {
		t.Error("an unavailable vibrator-only session was told to move in a scene")
	}
}
