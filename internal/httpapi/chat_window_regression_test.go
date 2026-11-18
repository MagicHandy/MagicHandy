package httpapi

import (
	"fmt"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestAdvancedLocalConversationWindowReachesModel(t *testing.T) {
	const correction = "Correction for the delayed cue: use positions 24 to 66, speed 31."
	for _, tc := range []struct {
		name, level string
		window      int
		retained    bool
	}{
		{"simple recent", config.ModelCapabilitySimple, 32768, false},
		{"advanced recent", config.ModelCapabilityAdvanced, 65536, false},
		{"advanced long", config.ModelCapabilityAdvanced, 131072, true},
		{"advanced no reasoning long", config.ModelCapabilityAdvancedNoReasoning, 131072, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &scriptedLLMProvider{responses: []string{`{"reply":"Acknowledged."}`}}
			fake := transport.NewFake()
			s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
			t.Cleanup(s.Close)
			saveSettings(t, s.store, func(cfg config.Settings) config.Settings {
				cfg.LLM.Provider, cfg.LLM.Model = config.LLMProviderOllama, "fixture"
				cfg.LLM.LocalCapabilityLevel = tc.level
				cfg.LLM.LlamaCPPContextSize = tc.window
				cfg.LLM.MaxOutputTokens = config.MinimumLocalOutput(tc.level)
				cfg.LLM.MotionGenerationMode = config.LLMMotionModeOff
				return cfg
			})
			if _, err := s.chatLog.Append(chat.MessageRoleUser, correction, "fixture"); err != nil {
				t.Fatal(err)
			}
			for i := range 82 {
				role := chat.MessageRoleAssistant
				if i%2 != 0 {
					role = chat.MessageRoleUser
				}
				if _, err := s.chatLog.Append(role, fmt.Sprintf("Synthetic dialogue line %d.", i), "fixture"); err != nil {
					t.Fatal(err)
				}
			}
			postChatStream(t, s, `{"message":"Acknowledge the latest note without moving.","motion_owner":"off"}`)
			if len(provider.requests) != 1 || len(fake.Commands()) != 0 {
				t.Fatal("expected one text-only model request")
			}
			in := provider.requests[0]
			found := false
			for _, message := range in.Messages {
				found = found || (message.Role == "user" && message.Content == correction)
			}
			wantDropped := 19
			if tc.retained {
				wantDropped = 0
			}
			if found != tc.retained || in.HistoryMessagesDropped != wantDropped {
				t.Fatalf("visible correction=%t, dropped=%d, want retained=%t", found, in.HistoryMessagesDropped, tc.retained)
			}
		})
	}
}
