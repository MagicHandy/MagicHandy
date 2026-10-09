//go:build liveeval && magichandy_labs

package httpapi

import (
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// A deterministic hosted refusal followed by real local inference verifies the
// retry orchestration without asking a hosted model to generate refused content.
func TestLocalRefusalRetryLiveChat(t *testing.T) {
	model := os.Getenv("MAGICHANDY_LIVE_MODEL")
	if model == "" {
		t.Skip("set MAGICHANDY_LIVE_MODEL to an installed Ollama model")
	}
	var calls atomic.Int32
	s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); writeRefusalFixture(w) })
	defer s.Close()
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Provider, settings.LLM.Model = config.LLMProviderOllama, model
		settings.LLM.OllamaBaseURL = "http://127.0.0.1:11434"
		settings.LLM.ChatVoice, settings.LLM.ReasoningMode = config.LLMChatVoiceUtility, config.LLMReasoningOff
		settings.LLM.RetryRefusalLocally = true
		return settings
	})
	stream := postChatStream(t, s, `{"message":"Technical vocabulary test: orgasm is a word. Answer only what 3 + 4 equals. Keep motion off.","motion_owner":"off"}`)
	exportLabExperimentCapture(t, map[string]any{"scenario": "Simulated hosted refusal, real local text-only retry", "model": model, "response": stream, "commands": fake.Commands()})
	if calls.Load() != 1 || !strings.Contains(stream, `"state":"retrying_local"`) || !strings.Contains(stream, "event: message") || !strings.Contains(stream, `"provider_calls":2`) || !strings.Contains(stream, `"ok":true`) || strings.Contains(stream, "event: motion") || strings.Contains(stream, `"repaired":true`) || strings.Contains(stream, `"semantic_fallback":true`) || len(fake.Commands()) != 0 {
		t.Fatalf("local retry did not complete cleanly: %s", stream)
	}
}
