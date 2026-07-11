package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLLMCacheKeyIgnoresPromptSet(t *testing.T) {
	settings := config.DefaultSettings().LLM
	settings.PromptSet = "prompt-a"
	first := llmCacheKey(settings, "runtime-a")

	settings.PromptSet = "prompt-b"
	if got := llmCacheKey(settings, "runtime-a"); got != first {
		t.Fatalf("cache key changed after prompt set update; provider should not reload for prompt-only changes")
	}

	settings.Model = "other-model"
	if got := llmCacheKey(settings, "runtime-a"); got == first {
		t.Fatal("cache key did not change after model update")
	}

	settings.Model = "local-model"
	if got := llmCacheKey(settings, "runtime-b"); got == first {
		t.Fatal("cache key did not change after managed runtime update")
	}
}

func TestSelectedLLMBaseURLKeepsManagedRunnerOnLoopback(t *testing.T) {
	settings := config.DefaultSettings().LLM
	settings.LlamaCPPBaseURL = "http://192.0.2.10:9000"
	if got := selectedLLMBaseURL(settings); got != config.DefaultLlamaCPPBaseURL {
		t.Fatalf("managed llama.cpp URL = %q, want %q", got, config.DefaultLlamaCPPBaseURL)
	}

	settings.LlamaCPPMode = config.LlamaCPPModeExternal
	if got := selectedLLMBaseURL(settings); got != "http://192.0.2.10:9000" {
		t.Fatalf("external llama.cpp URL = %q", got)
	}

	settings.Provider = config.LLMProviderOllama
	settings.OllamaBaseURL = "http://192.0.2.20:11434"
	if got := selectedLLMBaseURL(settings); got != settings.OllamaBaseURL {
		t.Fatalf("Ollama URL = %q", got)
	}
}
