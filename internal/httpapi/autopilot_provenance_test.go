package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

func TestAutopilotAnnouncementKeepsGeneratingModelAfterRoutingChanges(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	source := modes.SpeechSource{Provider: "chatgpt", Model: "gpt-6-sol"}
	// Saved local defaults differ from this completed conversation-model turn.
	announcement := server.autopilotAnnounce(t.Context(), "A check-in from the conversation model.", source)
	if !announcement.Published {
		t.Fatal("announcement not published")
	}
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 1 || messages[0].Diagnostics == nil {
		t.Fatal("diagnostics absent")
	}
	got := messages[0].Diagnostics
	if got.Provider != source.Provider || got.Model != source.Model {
		t.Fatalf("publication mislabeled generated response: %+v", got)
	}
}

func TestSpeechDecisionCapturesItsResolvedModel(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{`{"reply":"A brief check-in.","next":"normal"}`}}
	s := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(s.Close)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Provider = "ollama"
		settings.LLM.Model = "generation-model"
		return settings
	})
	decision, err := s.autopilotDecideSpeech(t.Context(), modes.DecisionInput{})
	if err != nil {
		t.Fatal(err)
	}
	if decision.SpeechSource.Model != "generation-model" || decision.SpeechSource.Provider != "ollama" {
		t.Fatalf("source missing: %+v", decision.SpeechSource)
	}
	saveSettings(t, s.store, func(settings config.Settings) config.Settings { settings.LLM.Model = "later-model"; return settings })
	s.autopilotAnnounce(t.Context(), decision.Say, decision.SpeechSource)
	messages, _, _ := getChatMessages(t, s, "")
	if len(messages) != 1 || messages[0].Diagnostics.Model != "generation-model" {
		t.Fatal("saved routing replaced the generation identity")
	}
}
