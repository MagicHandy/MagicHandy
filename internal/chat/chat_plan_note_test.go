package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func planNoteTestService(mode MotionMode, planNote bool, responses ...string) (Service, *scriptedProvider) {
	score := DefaultLayeredScore(40)
	if mode == MotionModeCreativeV2 {
		score = FreshCreativeV2Score(40)
	}
	state := MotionContext{Running: true, MotionMode: mode, Layered: &score, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80}
	caps := Capabilities{Motion: true, MotionMode: mode, AdvancedMotion: true, ChatPlanNote: planNote, PreserveConversationText: true}
	provider := &scriptedProvider{responses: responses}
	return Service{Provider: provider, Model: "scripted", Capabilities: &caps, MotionContext: &state, ReasoningMode: "off"}, provider
}

// The no-reasoning level asks for the private basis before edits in both continuous
// modes; the parser removes it, and the reply and motion are unchanged.
func TestChatPlanNoteWritesPrivateBasisBeforeEdits(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		edits := `{"composition":"manual","controls":{"speed_percent":{"op":"set","value":50}}}`
		if mode == MotionModeCreativeV2 {
			edits = `[{"speed_percent":50}]`
		}
		raw := `{"action":"update","basis":"Raise only the base speed to 50; manual controls keep the rest.","edits":` + edits + `,"reply":"Speed is now 50 percent."}`
		service, provider := planNoteTestService(mode, true, raw)
		result, err := service.Complete(t.Context(), Request{Message: "Go a bit faster, to 50."}, nil)
		if err != nil || result.Response.Motion == nil || result.Response.Motion.Layered == nil || result.Response.Motion.Layered.SpeedPercent != 50 {
			t.Fatalf("%s: planned reply rejected: %v %+v", mode, err, result.Response)
		}
		if strings.Contains(result.Response.Reply, "basis") || result.Response.Reply != "Speed is now 50 percent." {
			t.Fatalf("%s: private note reached the reply: %q", mode, result.Response.Reply)
		}
		request := provider.requests[0]
		if !strings.HasSuffix(request.Messages[0].Content, chatPlanGuide(mode)) {
			t.Fatalf("%s: guide does not close the system prompt", mode)
		}
		if (mode == MotionModeLayered) != strings.Contains(chatPlanGuide(mode), "alternate_ends") {
			t.Fatalf("%s: Layered checks offered to the wrong mode", mode)
		}
		var schema struct {
			OneOf []struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			} `json:"oneOf"`
		}
		if json.Unmarshal(request.JSONSchema, &schema) != nil || len(schema.OneOf) == 0 {
			t.Fatalf("%s: unexpected chat schema %s", mode, request.JSONSchema)
		}
		for _, branch := range schema.OneOf {
			if branch.Properties["basis"] == nil || !strings.Contains(strings.Join(branch.Required, ","), "basis") {
				t.Fatalf("%s: a chat branch lacks the required basis: %s", mode, request.JSONSchema)
			}
		}
	}
}

// Advanced requests stay byte-for-byte as before, and an unconstrained planned
// response without a basis still parses.
func TestChatPlanNoteLeavesAdvancedAndLegacyRepliesUnchanged(t *testing.T) {
	raw := `{"action":"update","edits":{"composition":"manual","controls":{"speed_percent":{"op":"set","value":50}}},"reply":"Speed is now 50 percent."}`
	advanced, provider := planNoteTestService(MotionModeLayered, false, raw)
	if _, err := advanced.Complete(t.Context(), Request{Message: "Go a bit faster, to 50."}, nil); err != nil {
		t.Fatal(err)
	}
	request := provider.requests[0]
	if strings.Contains(string(request.JSONSchema), `"basis"`) || strings.Contains(request.Messages[0].Content, chatPlanIntro) {
		t.Fatal("Advanced chat acquired the plan note")
	}
	planned, _ := planNoteTestService(MotionModeLayered, true, raw)
	if result, err := planned.Complete(t.Context(), Request{Message: "Go a bit faster, to 50."}, nil); err != nil || result.Response.Motion == nil {
		t.Fatalf("plan note rejected a reply without a basis: %v", err)
	}
	for _, invalid := range []string{
		`{"action":"update","basis":7,"edits":{},"reply":"No change."}`,
		`{"action":"update","basis":"a","basis":"b","edits":{},"reply":"No change."}`,
	} {
		if _, _, err := takeChatPlanNote(invalid); err == nil {
			t.Fatalf("accepted invalid planning metadata: %s", invalid)
		}
	}
}

// The note belongs to interactive chat only: Autopilot keeps its own intent,
// and motion-off chat sends no motion schema to extend.
func TestChatPlanNoteStaysOnInteractiveMotionChat(t *testing.T) {
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, ChatPlanNote: true}
	if !usesChatPlanNote(caps, false) || usesChatPlanNote(caps, true) {
		t.Fatal("the plan note must apply to chat and never to Autopilot")
	}
	for _, other := range []Capabilities{
		{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: false, ChatPlanNote: true},
		{Motion: false, MotionMode: MotionModeLayered, AdvancedMotion: true, ChatPlanNote: true},
		{Motion: true, MotionMode: MotionModePattern, AdvancedMotion: true, ChatPlanNote: true},
	} {
		if usesChatPlanNote(other, false) {
			t.Fatalf("the plan note escaped its contract: %+v", other)
		}
	}
	messages := []llm.Message{{Role: "system", Content: "System."}, {Role: "user", Content: "Hi."}}
	got := withChatPlanMessages(messages, MotionModeLayered)
	if messages[0].Content != "System." || !strings.HasSuffix(got[0].Content, chatPlanGuide(MotionModeLayered)) {
		t.Fatal("mutated the source messages or lost the guide")
	}
}

// A rejected planned response is repaired with the same contract and note.
func TestChatPlanNoteRepairKeepsThePlanningContract(t *testing.T) {
	rejected := `{"action":"update","basis":"Set speed.","edits":{"composition":"manual","controls":{"speed_percent":{"op":"set","op":"adjust","value":5}}},"reply":"Faster."}`
	repaired := `{"action":"update","basis":"Adjust speed by five; nothing else changes.","edits":{"composition":"manual","controls":{"speed_percent":{"op":"adjust","value":5}}},"reply":"A little faster."}`
	service, provider := planNoteTestService(MotionModeLayered, true, rejected, repaired)
	service.HostedResponseRepair = true
	result, err := service.Complete(t.Context(), Request{Message: "A little faster."}, nil)
	if err != nil || !result.Repaired || result.Response.Motion == nil || result.Response.Motion.Layered.SpeedPercent != 45 {
		t.Fatalf("planned repair failed: %v %+v", err, result)
	}
	if len(provider.requests) != 2 || !strings.Contains(string(provider.requests[1].JSONSchema), `"basis"`) {
		t.Fatal("repair dropped the planning contract")
	}
}
