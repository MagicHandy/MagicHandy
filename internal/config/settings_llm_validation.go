package config

import (
	"errors"
	"fmt"
	"net/url"
)

func validateLLMSettings(settings LLMSettings) error {
	if !oneOf(settings.Provider, LLMProviderLlamaCPP, LLMProviderOllama) {
		return fmt.Errorf("unknown LLM provider %q", settings.Provider)
	}
	if !oneOf(settings.LlamaCPPMode, LlamaCPPModeManaged, LlamaCPPModeExternal) {
		return fmt.Errorf("unknown llama.cpp mode %q", settings.LlamaCPPMode)
	}
	if !oneOf(settings.ManagedLoadPolicy, LLMManagedLoadStartup, LLMManagedLoadOnDemand) {
		return fmt.Errorf("unknown managed LLM load policy %q", settings.ManagedLoadPolicy)
	}
	if settings.LlamaCPPBaseURL == "" {
		return errors.New("llama.cpp base URL is required")
	}
	if settings.OllamaBaseURL == "" {
		return errors.New("ollama base URL is required")
	}
	if err := validateLLMBaseURL("llama.cpp", settings.LlamaCPPBaseURL); err != nil {
		return err
	}
	if err := validateLLMBaseURL("Ollama", settings.OllamaBaseURL); err != nil {
		return err
	}
	if !oneOfInt(settings.LlamaCPPContextSize, LlamaCPPContextSizes()...) {
		return fmt.Errorf("unsupported managed llama.cpp context size %d", settings.LlamaCPPContextSize)
	}
	if settings.Model == "" {
		return errors.New("LLM model is required")
	}
	// Prompt sets are dynamic (built-in templates plus user-created sets in
	// the prompt library), so config only requires a non-empty identifier;
	// the chat layer falls back to the bundled default if a selection is gone.
	if settings.PromptSet == "" {
		return errors.New("prompt set is required")
	}
	if settings.RequestTimeoutMillis < 1000 || settings.RequestTimeoutMillis > 300000 {
		return errors.New("LLM request timeout must be between 1000 and 300000 milliseconds")
	}
	if settings.MaxOutputTokens < 64 || settings.MaxOutputTokens > 4096 {
		return errors.New("LLM output limit must be between 64 and 4096 tokens")
	}
	if err := validateLLMBehaviorSettings(settings); err != nil {
		return err
	}
	if !oneOf(settings.UserAnatomy, LLMUserAnatomyPenis, LLMUserAnatomyVagina, LLMUserAnatomyCustom) {
		return fmt.Errorf("unknown LLM user anatomy %q", settings.UserAnatomy)
	}
	if len([]rune(settings.CustomAnatomy)) > MaxLLMCustomAnatomyChars {
		return fmt.Errorf("custom anatomy wording must be at most %d characters", MaxLLMCustomAnatomyChars)
	}
	if len([]rune(settings.PersonaDescription)) > MaxLLMPersonaDescriptionChars {
		return fmt.Errorf("persona description must be at most %d characters", MaxLLMPersonaDescriptionChars)
	}
	return nil
}

func validateLLMBehaviorSettings(settings LLMSettings) error {
	if !oneOf(settings.ReasoningMode, LLMReasoningAuto, LLMReasoningOff) {
		return fmt.Errorf("unknown LLM reasoning mode %q", settings.ReasoningMode)
	}
	if !oneOf(settings.MotionGenerationMode, LLMMotionModeDynamic, LLMMotionModePattern, LLMMotionModeLayered, LLMMotionModeCreativeV2, LLMMotionModeOff) {
		return fmt.Errorf("unknown LLM motion generation mode %q", settings.MotionGenerationMode)
	}
	if !ValidLLMChatVoice(settings.ChatVoice) {
		return fmt.Errorf("unknown LLM chat voice %q", settings.ChatVoice)
	}
	return nil
}

func validateLLMBaseURL(label, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return fmt.Errorf("%s base URL must be an absolute HTTP URL with a host", label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s base URL scheme must be http or https", label)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s base URL must not include userinfo", label)
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return fmt.Errorf("%s base URL must not include a query", label)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("%s base URL must not include a fragment", label)
	}
	return nil
}
