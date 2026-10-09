package config

import (
	"errors"
	"strings"
)

// Motion planning can retain local behavior or select separately billed APIs.
const (
	MotionPlannerLocal     = "local"
	MotionPlannerChatGPT   = "chatgpt"
	MotionPlannerDecisions = "decisions"
)

// MotionPlannerSettings assigns a model independently from its context policy.
// Credentials and verified accounts live outside settings and exports.
type MotionPlannerSettings struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ConnectionID  string `json:"connection_id,omitempty"`
	ContextPolicy string `json:"context_policy"`
}

func normalizeMotionPlanner(settings MotionPlannerSettings) MotionPlannerSettings {
	settings.Provider = strings.TrimSpace(settings.Provider)
	settings.Model = strings.TrimSpace(settings.Model)
	if settings.Provider == "" {
		settings.Provider = "conversation"
	}
	if settings.ContextPolicy == "" {
		settings.ContextPolicy = ContextConversation
	}
	return settings
}

func validateMotionPlanner(settings MotionPlannerSettings) error {
	settings = normalizeMotionPlanner(settings)
	if !oneOf(settings.Provider, "conversation", "connection", MotionPlannerLocal, MotionPlannerChatGPT, MotionPlannerDecisions) {
		return errors.New("unknown motion planning provider")
	}
	if !oneOf(settings.ContextPolicy, ContextConversation, ContextTechnical) {
		return errors.New("unknown motion context sharing policy")
	}
	if len(settings.Model) > 160 {
		return errors.New("motion model identifier is too long")
	}
	if settings.Provider == MotionPlannerChatGPT && settings.Model == "" {
		return errors.New("select an available ChatGPT motion model")
	}
	return nil
}
