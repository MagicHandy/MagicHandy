package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Hosted providers and context policies are independent of saved local defaults.
const (
	LLMProviderChatGPT    = "chatgpt"
	LLMProviderOpenAI     = "openai"
	LLMProviderOpenRouter = "openrouter"
	LLMProviderCompatible = "compatible"
	ContextConversation   = "conversation"
	ContextTechnical      = "technical"
)

// ModelConnection separates endpoint/protocol/model capabilities from roles.
// Credentials are bound to this ID, provider, and endpoint in host storage.
// The reserved local connection uses the existing local fields losslessly.
type ModelConnection struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Provider            string   `json:"provider"`
	BaseURL             string   `json:"base_url"`
	Model               string   `json:"model"`
	ReasoningEffort     string   `json:"reasoning_effort,omitempty"`
	OutputMode          string   `json:"output_mode"`
	SupportedParameters []string `json:"supported_parameters,omitempty"`
	AllowFallbacks      bool     `json:"allow_fallbacks"`
	AllowedProviders    []string `json:"allowed_providers,omitempty"`
	DataCollection      string   `json:"data_collection"`
	ZeroDataRetention   bool     `json:"zero_data_retention"`
	NoAuthentication    bool     `json:"no_authentication,omitempty"`
}

var connectionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidConnectionID excludes the reserved local connection and unsafe IDs.
func ValidConnectionID(id string) bool { return connectionIDPattern.MatchString(id) && id != "local" }

// Normalize applies endpoint and routing defaults without introducing a secret.
func (c ModelConnection) Normalize() ModelConnection {
	c.Name = strings.TrimSpace(c.Name)
	c.Model = strings.TrimSpace(c.Model)
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.OutputMode == "" {
		c.OutputMode = "auto"
	}
	if c.DataCollection == "" {
		c.DataCollection = "deny"
	}
	switch c.Provider {
	case LLMProviderChatGPT, LLMProviderOpenAI:
		c.BaseURL = "https://api.openai.com/v1"
	case LLMProviderOpenRouter:
		c.BaseURL = "https://openrouter.ai/api/v1"
	}
	return c
}

// Validate checks the public connection and capability policy.
func (c ModelConnection) Validate() error {
	if !ValidConnectionID(c.ID) || len(c.Name) == 0 || len(c.Name) > 100 || len(c.Model) > 160 {
		return errors.New("connection needs a stable ID, bounded name and model")
	}
	if !oneOf(c.Provider, LLMProviderChatGPT, LLMProviderOpenAI, LLMProviderOpenRouter, LLMProviderCompatible) {
		return errors.New("unknown model connection provider")
	}
	if err := c.validateEndpoint(); err != nil {
		return err
	}
	if !oneOf(c.OutputMode, "auto", "strict", "json", "prompt") || !oneOf(c.DataCollection, "deny", "allow") || len(c.AllowedProviders) > 16 {
		return errors.New("invalid model capability or routing policy")
	}
	if c.NoAuthentication && c.Provider != LLMProviderCompatible {
		return errors.New("this provider requires its own credentials")
	}
	if err := c.validateReasoning(); err != nil {
		return err
	}
	for _, parameter := range c.SupportedParameters {
		if !oneOf(parameter, "temperature", "top_p", "max_tokens", "max_completion_tokens", "response_format", "structured_outputs") {
			return errors.New("unsupported provider parameter")
		}
	}
	for _, provider := range c.AllowedProviders {
		if len(provider) == 0 || len(provider) > 120 {
			return errors.New("invalid OpenRouter provider restriction")
		}
	}
	return nil
}

func (c ModelConnection) validateEndpoint() error {
	if err := validateLLMBaseURL("model connection", c.BaseURL); err != nil {
		return err
	}
	parsed, _ := url.Parse(c.BaseURL)
	if parsed.Scheme != "https" && !net.ParseIP(parsed.Hostname()).IsLoopback() && parsed.Hostname() != "localhost" {
		return errors.New("remote model credentials require HTTPS; HTTP is allowed only on loopback")
	}
	return nil
}

func (c ModelConnection) validateReasoning() error {
	if !oneOf(c.ReasoningEffort, "", "none", "minimal", "low", "medium", "high", "xhigh", "max") {
		return errors.New("invalid model reasoning effort")
	}
	if c.ReasoningEffort != "" && c.Provider != LLMProviderChatGPT && c.Provider != LLMProviderOpenAI {
		return errors.New("reasoning effort requires a ChatGPT or OpenAI connection")
	}
	return nil
}

// ConnectionByID resolves a named hosted connection from saved settings.
func (s LLMSettings) ConnectionByID(id string) (ModelConnection, bool) {
	index := slices.IndexFunc(s.Connections, func(c ModelConnection) bool { return c.ID == id })
	if index < 0 {
		return ModelConnection{}, false
	}
	return s.Connections[index], true
}

// ConversationSettings resolves the role without changing saved local choices.
func (s LLMSettings) ConversationSettings() LLMSettings {
	if s.ActiveConnection != nil {
		return s
	}
	if connection, ok := s.ConnectionByID(s.ConversationConnectionID); ok {
		return s.WithConnection(connection)
	}
	return s
}

// WithConnection creates a runtime view, leaving saved local defaults intact.
func (s LLMSettings) WithConnection(connection ModelConnection) LLMSettings {
	connection = connection.Normalize()
	s.ActiveConnection = &connection
	s.Provider, s.Model = connection.Provider, connection.Model
	return s
}

// WithModel selects a temporary model without changing the endpoint binding.
func (s LLMSettings) WithModel(model string) LLMSettings {
	s.Model = model
	if s.ActiveConnection != nil {
		connection := *s.ActiveConnection
		connection.Model = model
		s.ActiveConnection = &connection
	}
	return s
}

// PlanningSettings resolves the explicitly assigned motion model role.
func (s LLMSettings) PlanningSettings() (LLMSettings, error) {
	s.RequestRole = "motion"
	if s.MotionPlanner.Provider == "connection" {
		if s.MotionPlanner.ConnectionID == "local" {
			s.ConversationConnectionID = ""
			s.ActiveConnection = nil
			return s, nil
		}
		connection, ok := s.ConnectionByID(s.MotionPlanner.ConnectionID)
		if !ok {
			return LLMSettings{}, errors.New("selected motion connection is unavailable")
		}
		return s.WithConnection(connection), nil
	}
	if s.MotionPlanner.Provider == MotionPlannerLocal {
		s.ConversationConnectionID = ""
		s.ActiveConnection = nil
		return s, nil
	}
	if s.MotionPlanner.Provider == MotionPlannerChatGPT {
		return s.WithConnection(ModelConnection{ID: "chatgpt", Name: "ChatGPT", Provider: LLMProviderChatGPT, Model: s.MotionPlanner.Model}), nil
	}
	return s.ConversationSettings(), nil
}

func validateModelConnections(s LLMSettings) error {
	if len(s.Connections) > 16 {
		return errors.New("at most sixteen model connections can be saved")
	}
	seen := map[string]bool{}
	for _, connection := range s.Connections {
		if seen[connection.ID] {
			return errors.New("model connection IDs must be unique")
		}
		seen[connection.ID] = true
		if err := connection.Validate(); err != nil {
			return err
		}
	}
	if s.ConversationConnectionID != "" && s.ConversationConnectionID != "local" && !seen[s.ConversationConnectionID] {
		return errors.New("selected conversation connection is unavailable")
	}
	if s.MotionPlanner.Provider == "connection" && s.MotionPlanner.ConnectionID != "local" && !seen[s.MotionPlanner.ConnectionID] {
		return fmt.Errorf("selected motion connection is unavailable")
	}
	return nil
}

// IsHosted identifies runtime settings that use a hosted protocol adapter.
func (s LLMSettings) IsHosted() bool {
	return oneOf(s.Provider, LLMProviderChatGPT, LLMProviderOpenAI, LLMProviderOpenRouter, LLMProviderCompatible)
}
