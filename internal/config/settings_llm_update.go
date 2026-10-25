package config

import (
	"fmt"
	"strings"
)

func applyLLMUpdate(current LLMSettings, update LLMUpdate) (LLMSettings, error) {
	level, contextSize, maxOutputTokens, err := localCapabilityUpdate(current, update)
	if err != nil {
		return LLMSettings{}, err
	}
	repairEnabled := current.HostedResponseRepairEnabled
	if update.HostedResponseRepairEnabled != nil {
		enabled := *update.HostedResponseRepairEnabled
		repairEnabled = &enabled
	}
	retryRefusalLocally := current.RetryRefusalLocally
	if update.RetryRefusalLocally != nil {
		retryRefusalLocally = *update.RetryRefusalLocally
	}
	connections, conversation := current.Connections, current.ConversationConnectionID
	if update.Connections != nil {
		connections = *update.Connections
	}
	if update.ConversationConnectionID != nil {
		conversation = *update.ConversationConnectionID
	}
	planner := current.MotionPlanner
	reviewer := current.MotionReviewer
	if update.MotionReviewer != nil {
		reviewer = *update.MotionReviewer
	}
	if update.MotionPlanner != nil {
		planner = *update.MotionPlanner
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
	replyLength := current.ReplyLength
	if update.ReplyLength != nil {
		replyLength = *update.ReplyLength
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
	motionGenerationMode := current.MotionGenerationMode
	if update.MotionGenerationMode != nil {
		motionGenerationMode = *update.MotionGenerationMode
	}
	capabilities := current.MotionCapabilities
	if update.MotionCapabilities != nil {
		copied := *update.MotionCapabilities
		capabilities = &copied
	}
	return normalizeLLMStrings(LLMSettings{
		Connections:                 connections,
		ConversationConnectionID:    conversation,
		RetryRefusalLocally:         retryRefusalLocally,
		HostedResponseRepairEnabled: repairEnabled,
		MotionPlanner:               planner,
		MotionReviewer:              reviewer,
		Provider:                    update.Provider,
		LlamaCPPMode:                update.LlamaCPPMode,
		ManagedLoadPolicy:           managedLoadPolicy,
		LlamaCPPBaseURL:             update.LlamaCPPBaseURL,
		LlamaCPPContextSize:         contextSize,
		LocalCapabilityLevel:        level,
		OllamaBaseURL:               update.OllamaBaseURL,
		OllamaModelsPath:            update.OllamaModelsPath,
		Model:                       update.Model,
		PromptSet:                   update.PromptSet,
		RequestTimeoutMillis:        update.RequestTimeoutMillis,
		MaxOutputTokens:             maxOutputTokens,
		ReasoningMode:               reasoningMode,
		ChatVoice:                   chatVoice,
		ReplyLength:                 replyLength,
		UserAnatomy:                 userAnatomy,
		CustomAnatomy:               customAnatomy,
		PersonaDescription:          personaDescription,
		MotionGenerationMode:        motionGenerationMode,
		MotionCapabilities:          capabilities,
	}), nil
}

// A capability change may raise the required allocation, while omitted fields
// from older clients preserve the user's saved level and larger budgets.
func localCapabilityUpdate(current LLMSettings, update LLMUpdate) (string, int, int, error) {
	level := current.LocalCapabilityLevel
	if update.LocalCapabilityLevel != nil {
		level = *update.LocalCapabilityLevel
	}
	level = strings.ToLower(strings.TrimSpace(level))
	if update.LlamaCPPContextSize != nil && !oneOfInt(*update.LlamaCPPContextSize, LlamaCPPContextSizes()...) {
		return "", 0, 0, fmt.Errorf("unsupported managed llama.cpp context size %d", *update.LlamaCPPContextSize)
	}
	contextSize := current.LlamaCPPContextSize
	if update.LlamaCPPContextSize != nil {
		contextSize = *update.LlamaCPPContextSize
	}
	contextSize = max(contextSize, MinimumLocalContext(level))
	maxOutputTokens := current.MaxOutputTokens
	if update.MaxOutputTokens != nil {
		maxOutputTokens = *update.MaxOutputTokens
	}
	if level == ModelCapabilityIntermediate || level == ModelCapabilityAdvanced {
		maxOutputTokens = max(maxOutputTokens, MinimumLocalOutput(level))
	}
	return level, contextSize, maxOutputTokens, nil
}
