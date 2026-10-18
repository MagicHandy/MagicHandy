package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
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
		caps := chatCapabilities(settings, nil)
		if got := caps.HostedMotion; got != tc.hosted {
			t.Errorf("provider %s operations=%v want=%v", tc.provider, got, tc.hosted)
		}
		if got := autopilotPromptContext(modes.DecisionInput{}, caps).Hosted; got != tc.hosted {
			t.Errorf("provider %s planner contract=%v", tc.provider, got)
		}
	}
}
