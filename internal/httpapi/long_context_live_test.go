//go:build liveeval && magichandy_labs

package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestLongContextLiveApp(t *testing.T) {
	directory, output := os.Getenv("MAGICHANDY_HOSTED_DATA_DIR"), os.Getenv("MAGICHANDY_CONTEXT_APP_REPORT")
	if directory == "" || output == "" {
		t.Skip("explicit signed-in profile and report required")
	}
	auth, err := openaiauth.Open(openaiauth.Options{DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auth.Close)
	token, err := auth.ActiveTokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	provider, err := llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{PlanUsage: true, Model: "gpt-6-sol", ReasoningEffort: "low", ContextSize: 131072, Token: token, Timeout: 60 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{LLMProvider: provider, Transport: fake, MotionTransport: fake})
	connection := config.ModelConnection{ID: "review", Name: "Review", Provider: config.LLMProviderChatGPT, Model: "gpt-6-sol", ReasoningEffort: "low", ContextWindow: 131072}.Normalize()
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Connections = []config.ModelConnection{connection}
		settings.LLM.ConversationConnectionID = connection.ID
		settings.LLM.MotionGenerationMode = config.LLMMotionModeOff
		settings.LLM.ChatVoice = "utility"
		return settings
	})
	for _, message := range []string{"Our story takes place aboard Juniper. Inez holds a copper compass; meet at the east pier.", "Correction: Elian now holds the copper compass. Meet at the west quay instead."} {
		if _, err := s.chatLog.Append(chat.MessageRoleUser, message, "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 220; i++ {
		role := chat.MessageRoleUser
		if i%2 == 1 {
			role = chat.MessageRoleAssistant
		}
		line := fmt.Sprintf("Journal %d: %s", i, strings.Repeat("The travelers follow the quiet footpath beneath the trees. ", 5))
		if _, err := s.chatLog.Append(role, line, "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now()
	stream := postChatStream(t, s, `{"message":"Continue the story in two sentences, naming the ship, the compass material, its current holder, and our corrected meeting place. Keep motion off."}`)
	passed := strings.Contains(stream, `"ok":true`) && strings.Contains(stream, `"provider_calls":1`) && !strings.Contains(stream, `"repaired":true`) && !strings.Contains(stream, `"semantic_fallback":true`) && len(fake.Commands()) == 0
	for _, fact := range []string{"juniper", "copper", "elian", "west quay"} {
		passed = passed && strings.Contains(strings.ToLower(stream), fact)
	}
	report, _ := json.MarshalIndent(map[string]any{"model": "gpt-6-sol", "history_messages": 222, "elapsed_ms": time.Since(started).Milliseconds(), "passed": passed, "transport_commands": len(fake.Commands()), "stream": stream}, "", "  ")
	if err := os.WriteFile(output, report, 0600); err != nil {
		t.Fatal(err)
	}
	if !passed {
		t.Fatal("long-context production chat failed; see explicit synthetic report")
	}
	t.Logf("222 stored messages -> one hosted call -> correct continuous story; no motion; %d ms", time.Since(started).Milliseconds())
}
