package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLLMCacheKeyIgnoresPromptSet(t *testing.T) {
	settings := config.DefaultSettings().LLM
	settings.PromptSet = "prompt-a"
	first := llmCacheKey(settings)

	settings.PromptSet = "prompt-b"
	if got := llmCacheKey(settings); got != first {
		t.Fatalf("cache key changed after prompt set update; provider should not reload for prompt-only changes")
	}

	settings.Model = "other-model"
	if got := llmCacheKey(settings); got == first {
		t.Fatal("cache key did not change after model update")
	}
}
