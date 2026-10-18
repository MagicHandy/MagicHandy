package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLayeredOperationsFollowResolvedProvider(t *testing.T) {
	for _, tc := range []struct {
		provider string
		hosted   bool
	}{
		{config.LLMProviderOllama, false}, {config.LLMProviderLlamaCPP, false},
		{config.LLMProviderChatGPT, true}, {config.LLMProviderOpenAI, true},
		{config.LLMProviderOpenRouter, true}, {config.LLMProviderCompatible, true},
	} {
		settings := config.DefaultSettings().LLM
		settings.Provider = tc.provider
		settings.LlamaCPPMode = config.LlamaCPPModeExternal
		settings.MotionGenerationMode = "layered"
		if got := chatCapabilities(settings, nil).LayeredControlOperations; got != tc.hosted {
			t.Errorf("provider %s operations=%v want=%v", tc.provider, got, tc.hosted)
		}
	}
}
