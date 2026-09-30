package chat

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestChatOnlyTurnDoesNotTeachDisabledMotion(t *testing.T) {
	for _, holder := range []MotionHolder{MotionHolderSettings, MotionHolderVideoScript, MotionHolderVideoOff} {
		t.Run(string(holder), func(t *testing.T) {
			provider := &scriptedProvider{responses: []string{`{"reply":"Hello."}`}}
			capabilities := Capabilities{MotionHolder: holder}
			service := Service{Provider: provider, Capabilities: &capabilities}
			result, err := service.Complete(t.Context(), Request{Message: "Hello", History: []llm.Message{
				{Role: "user", Content: "Start slowly"},
				{Role: "assistant", Content: `{"reply":"Starting slowly.","motion":{"action":"start","speed_percent":20}}`},
				{Role: "assistant", Content: `{"reply":"A different character.","edits":[{"set":{"speed":25}}]}`},
				{Role: "assistant", Content: "Still here."},
			}}, nil)
			if err != nil || result.Malformed || result.Repaired || result.Response.Motion != nil {
				t.Fatalf("chat-only result = %+v, err=%v", result, err)
			}
			request := provider.requests[0]
			for _, message := range request.Messages {
				if message.Role == "user" {
					continue
				}
				for _, command := range []string{`"action":`, `motion.action`, `"edits":`, `"speed_percent":`} {
					if strings.Contains(message.Content, command) {
						t.Errorf("%s teaches disabled command %q: %s", message.Role, command, message.Content)
					}
				}
			}
			for i, want := range []string{"Starting slowly.", "A different character.", "Still here."} {
				if got := request.Messages[i+2].Content; got != want {
					t.Errorf("history[%d] = %q, want preserved speech %q", i, got, want)
				}
			}
		})
	}
}

func TestChatOnlySchemaMatchesCapabilities(t *testing.T) {
	for _, mood := range []bool{false, true} {
		var schema struct {
			Properties           map[string]json.RawMessage `json:"properties"`
			Required             []string                   `json:"required"`
			AdditionalProperties bool                       `json:"additionalProperties"`
		}
		if err := json.Unmarshal(PatternResponseSchema(nil, Capabilities{MoodTracking: mood}, nil), &schema); err != nil {
			t.Fatal(err)
		}
		wantFields := 1
		if mood {
			wantFields++
		}
		if len(schema.Properties) != wantFields || schema.Properties["reply"] == nil ||
			(schema.Properties["new_mood"] != nil) != mood || schema.AdditionalProperties ||
			len(schema.Required) != 1 || schema.Required[0] != "reply" {
			t.Fatalf("schema disagrees with chat-only capabilities (mood=%v): %+v", mood, schema)
		}
	}
}

func TestVideoChatKeepsSelectedVoiceAndLanguage(t *testing.T) {
	for _, prompt := range BuiltinPromptSets() {
		for _, voice := range []VoiceLevel{VoiceUtility, VoiceWarm, VoiceIntimate, VoiceExplicit} {
			for _, holder := range []MotionHolder{MotionHolderVideoScript, MotionHolderVideoOff} {
				capabilities := Capabilities{Voice: voice, MotionHolder: holder}
				context := &ConversationContext{PersonaName: "Rae", PersonaDescription: "Reserved, calm and precise."}
				composition := composePrompt(prompt, nil, nil, capabilities, nil, context)
				sections := make(map[string]string)
				for _, section := range composition.Sections {
					sections[section.ID] = section.Text
				}
				locale := promptLocaleForID(prompt.ID)
				if sections["voice_identity"] != voiceIdentityInstructionsForLocale(locale, voice) ||
					sections["voice_check"] != finalVoiceCheckForLocale(locale, voice) ||
					sections["language_reminder"] != replyLanguageReminderForPromptID(prompt.ID) {
					t.Errorf("video holder %q replaced voice %q or language %q", holder, voice, prompt.ID)
				}
				if voice != VoiceUtility && (!strings.Contains(sections["conversation_context"], context.PersonaName) ||
					!strings.Contains(sections["conversation_context"], context.PersonaDescription)) {
					t.Errorf("video holder %q lost the selected persona", holder)
				}
				if strings.Contains(sections["video_motion"], `{"reply":`) {
					t.Errorf("video holder %q teaches a fixed reply over the selected voice/language", holder)
				}
				if strings.Contains(composition.Prompt, "script moves the device right now") ||
					strings.Contains(composition.Prompt, "that script is moving the device") {
					t.Error("source selection alone is incorrectly presented as running motion")
				}
			}
		}
	}
}

func TestAutopilotSpeechRepairPreservesSelectedLanguage(t *testing.T) {
	for _, prompt := range BuiltinPromptSets() {
		provider := &scriptedProvider{responses: []string{"not json", `{"reply":"A repaired reply.","next":"normal"}`}}
		service := AutopilotService{Provider: provider, Prompt: prompt}
		if _, err := service.Complete(t.Context(), AutopilotKindSpeech, Request{Message: "Check in briefly", History: []llm.Message{
			{Role: "assistant", Content: `{"reply":"Hello.","motion":{"action":"none"}}`},
		}}); err != nil {
			t.Fatal(err)
		}
		request := provider.requests[1]
		if request.Messages[1].Content != "Hello." {
			t.Error("speech repair retained historical motion commands")
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(request.JSONSchema, &schema); err != nil || len(schema.Properties) != 2 ||
			schema.Properties["reply"] == nil || schema.Properties["next"] == nil {
			t.Fatalf("speech repair schema does not match the chat-only contract: %s, err=%v", request.JSONSchema, err)
		}
		repair := request.Messages[len(request.Messages)-1].Content
		if !strings.Contains(repair, repairLanguageInstruction(prompt.ID)) {
			t.Errorf("repair lost the selected language %q: %s", prompt.ID, repair)
		}
	}
	if repair := autopilotRepairPrompt(PromptSetIDSpanish, AutopilotKindMotion, errors.New("invalid next")); strings.Contains(repair, `The "reply" value`) {
		t.Error("motion-only repair requested speech outside its contract")
	}
}
