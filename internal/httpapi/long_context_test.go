package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLongConversationReachesHostedChatBeyondOldStorageAndPromptLimits(t *testing.T) {
	captured := make(chan []llm.Message, 2)
	s, fake, connection := hostedFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		captured <- body.Messages
		writeHostedFixtureReply(w, `{"reply":"The story continues."}`)
	})
	connection.ContextWindow = 131072
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Connections = []config.ModelConnection{connection}
		return settings
	})
	first := llm.Message{Role: "user", Content: "Keep this original detail verbatim: café 日本語.\nA long-running story begins here."}
	if _, err := s.chatLog.Append(first.Role, first.Content, "fixture"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 219; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		if _, err := s.chatLog.Append(role, fmt.Sprint(i)+strings.Repeat(" continued dialogue", 17), "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	response := postChatStream(t, s, `{"message":"Continue this story. Keep motion off."}`)
	if !strings.Contains(response, `"reply":"The story continues."`) || strings.Contains(response, "event: motion") || strings.Contains(response, `"repaired":true`) {
		t.Fatal(response)
	}
	messages := <-captured
	if !slices.Contains(messages, first) || len(messages) != 222 {
		t.Fatal("an upstream gate still cut long history", len(messages))
	}
	if len(fake.Commands()) != 0 {
		t.Fatal("text-only request moved a device")
	}
	assertLongContextRouting(t, s, first)
}

func assertLongContextRouting(t *testing.T, s *Server, first llm.Message) {
	t.Helper()
	settings, _ := s.store.Snapshot()
	id, _ := s.chatLog.ActiveSessionID()
	local, err := s.loadInteractiveChatPromptContext(t.Context(), id, settings.LLM.LocalSettings())
	if err != nil || len(local.History) != chat.PromptHistoryLimit || local.Capabilities.PreserveConversationText {
		t.Fatal("local context widened", len(local.History), err)
	}
	hosted := settings.LLM.ConversationSettings()
	planning, _, _, err := s.autopilotPersonalization(t.Context(), hosted, false)
	if err != nil || !slices.Contains(planning.History, first) {
		t.Fatal("Autopilot lost the longer conversation", err)
	}
	technical, _, memories, err := s.autopilotPersonalization(t.Context(), hosted, true)
	if err != nil || len(technical.History) > 0 || technical.ConversationContext != nil || len(memories) > 0 {
		t.Fatal("technical planner acquired private history", err)
	}
	if budget := chatPromptBudget(hosted); budget.ContextSize != 131072 {
		t.Fatal("composition budget does not match provider window")
	}
}
