package config

import "fmt"

func applyLLMUpdate(current LLMSettings, update LLMUpdate) (LLMSettings, error) {
	if update.LlamaCPPContextSize != nil && !oneOfInt(*update.LlamaCPPContextSize, LlamaCPPContextSizes()...) {
		return LLMSettings{}, fmt.Errorf("unsupported managed llama.cpp context size %d", *update.LlamaCPPContextSize)
	}
	contextSize := current.LlamaCPPContextSize
	if update.LlamaCPPContextSize != nil {
		contextSize = *update.LlamaCPPContextSize
	}
	maxOutputTokens := current.MaxOutputTokens
	if update.MaxOutputTokens != nil {
		maxOutputTokens = *update.MaxOutputTokens
	}
	reasoningMode := current.ReasoningMode
	if update.ReasoningMode != nil {
		reasoningMode = *update.ReasoningMode
	}
	managedLoadPolicy := current.ManagedLoadPolicy
	if update.ManagedLoadPolicy != nil {
		managedLoadPolicy = *update.ManagedLoadPolicy
	}
	chatVoice := current.ChatVoice
	if update.ChatVoice != nil {
		chatVoice = *update.ChatVoice
	}
	userAnatomy := current.UserAnatomy
	if update.UserAnatomy != nil {
		userAnatomy = *update.UserAnatomy
	}
	customAnatomy := current.CustomAnatomy
	if update.CustomAnatomy != nil {
		customAnatomy = *update.CustomAnatomy
	}
	personaDescription := current.PersonaDescription
	if update.PersonaDescription != nil {
		personaDescription = *update.PersonaDescription
	}
	capabilities := current.MotionCapabilities
	if update.MotionCapabilities != nil {
		copied := *update.MotionCapabilities
		capabilities = &copied
	}
	return normalizeLLMStrings(LLMSettings{
		Provider:             update.Provider,
		LlamaCPPMode:         update.LlamaCPPMode,
		ManagedLoadPolicy:    managedLoadPolicy,
		LlamaCPPBaseURL:      update.LlamaCPPBaseURL,
		LlamaCPPContextSize:  contextSize,
		OllamaBaseURL:        update.OllamaBaseURL,
		OllamaModelsPath:     update.OllamaModelsPath,
		Model:                update.Model,
		PromptSet:            update.PromptSet,
		RequestTimeoutMillis: update.RequestTimeoutMillis,
		MaxOutputTokens:      maxOutputTokens,
		ReasoningMode:        reasoningMode,
		ChatVoice:            chatVoice,
		UserAnatomy:          userAnatomy,
		CustomAnatomy:        customAnatomy,
		PersonaDescription:   personaDescription,
		MotionCapabilities:   capabilities,
	}), nil
}
