package chat

import (
	"strings"
	"testing"
)

// A chat beside an open video is chat-only while the video's script or the
// viewer's Off choice owns the device, and it must say who drives rather than
// send the person to Settings.
func TestChatOnlyContractNamesWhoDrivesTheDevice(t *testing.T) {
	if got := contractInstructions(Capabilities{}); got != contractChatOnly {
		t.Fatalf("settings-off contract changed:\n%s", got)
	}
	for _, test := range []struct {
		holder MotionHolder
		reason string
	}{
		{MotionHolderVideoScript, chatOnlyVideoScriptReason},
		{MotionHolderVideoOff, chatOnlyVideoOffReason},
	} {
		for _, mood := range []bool{false, true} {
			contract := contractInstructions(Capabilities{MotionHolder: test.holder, MoodTracking: mood})
			if !strings.Contains(contract, test.reason) || strings.Contains(contract, "switched off in Settings") {
				t.Errorf("holder %q (mood %v) contract:\n%s", test.holder, mood, contract)
			}
			if mood && !strings.Contains(contract, `"new_mood"`) {
				t.Errorf("holder %q lost the mood field:\n%s", test.holder, contract)
			}
		}
	}
}

// The note sits after the contract, just before the final guard, and is absent
// whenever the chat itself may move the device.
func TestVideoMotionNoteClosesThePromptWhenTheVideoDrives(t *testing.T) {
	for _, holder := range []MotionHolder{MotionHolderVideoScript, MotionHolderVideoOff} {
		composition := composePrompt(PromptSet{}, nil, nil, Capabilities{MotionHolder: holder}, nil, nil)
		ids := make([]string, 0, len(composition.Sections))
		for _, section := range composition.Sections {
			ids = append(ids, section.ID)
		}
		if len(ids) < 2 || ids[len(ids)-2] != "video_motion" || ids[len(ids)-1] != "output_guard" {
			t.Fatalf("holder %q sections = %v", holder, ids)
		}
	}
	for _, capabilities := range []Capabilities{{}, {Motion: true, MotionHolder: MotionHolderVideoScript}} {
		if note := videoMotionNote(capabilities); note != "" {
			t.Fatalf("note for %+v = %q", capabilities, note)
		}
	}
}
