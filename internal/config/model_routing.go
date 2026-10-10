package config

import "net/url"

// ModelRoute describes a saved destination, not credential or inference readiness.
// It is projected from the same runtime resolution used by chat and Autopilot.
type ModelRoute struct {
	Kind          string `json:"kind"`
	Provider      string `json:"provider"`
	ConnectionID  string `json:"connection_id,omitempty"`
	Model         string `json:"model"`
	EndpointHost  string `json:"endpoint_host,omitempty"`
	ContextPolicy string `json:"context_policy"`
	State         string `json:"state"`
}

// ModelRoutingSnapshot is the public projection of both saved model roles.
type ModelRoutingSnapshot struct {
	Chat       ModelRoute  `json:"chat"`
	Autopilot  ModelRoute  `json:"autopilot"`
	LocalRetry *ModelRoute `json:"local_retry,omitempty"`
}

// ModelRouting reports resolved destinations without exposing credentials.
func (s LLMSettings) ModelRouting() ModelRoutingSnapshot {
	chat := describeModelRoute(s.ConversationSettings())
	chat.ContextPolicy = ContextConversation
	planner, err := s.PlanningSettings()
	motion := describeModelRoute(planner)
	if err != nil {
		motion = ModelRoute{Kind: "unavailable", State: "unavailable", ConnectionID: s.MotionPlanner.ConnectionID}
	}
	motion.ContextPolicy = normalizeMotionPlanner(s.MotionPlanner).ContextPolicy
	if s.MotionPlanner.Provider == MotionPlannerDecisions {
		motion = ModelRoute{Kind: "decisions", Provider: LLMProviderOpenAI, EndpointHost: "api.openai.com", ContextPolicy: ContextTechnical, State: "configured"}
	}
	snapshot := ModelRoutingSnapshot{Chat: chat, Autopilot: motion}
	if s.RetryRefusalLocally && s.ConversationSettings().IsHosted() {
		local := describeModelRoute(s.LocalSettings())
		local.ContextPolicy = ContextConversation
		snapshot.LocalRetry = &local
	}
	return snapshot
}

func describeModelRoute(s LLMSettings) ModelRoute {
	route := ModelRoute{Kind: "local", Provider: s.Provider, Model: s.Model, State: "configured"}
	endpoint := s.LlamaCPPBaseURL
	if s.Provider == LLMProviderOllama {
		endpoint = s.OllamaBaseURL
	}
	if s.ActiveConnection != nil {
		route.Kind, route.ConnectionID = "hosted", s.ActiveConnection.ID
		endpoint = s.ActiveConnection.BaseURL
	}
	if !oneOf(s.Provider, LLMProviderLlamaCPP, LLMProviderOllama, LLMProviderChatGPT, LLMProviderOpenAI, LLMProviderOpenRouter, LLMProviderCompatible) {
		route.Kind, route.State = "unavailable", "unavailable"
		endpoint = ""
	} else if s.IsHosted() {
		route.Kind = "hosted"
		if s.ActiveConnection == nil {
			route.State = "unavailable"
		}
	}
	if route.State != "unavailable" && s.Model == "" {
		route.State = "incomplete"
	}
	if parsed, err := url.Parse(endpoint); err == nil {
		route.EndpointHost = parsed.Host
	}
	return route
}
