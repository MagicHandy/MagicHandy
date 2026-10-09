package config

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLocalRetrySettingsWriteRoundTrip(t *testing.T) {
	settings := DefaultSettings().LLM
	settings.RetryRefusalLocally = true
	payload, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var update LLMUpdate
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&update); err != nil {
		t.Fatal(err)
	}
	next, err := applyLLMUpdate(DefaultSettings().LLM, update)
	if err != nil || !next.RetryRefusalLocally {
		t.Fatalf("retry setting lost: %v", err)
	}
	update.RetryRefusalLocally = nil
	next, err = applyLLMUpdate(next, update)
	if err != nil || !next.RetryRefusalLocally {
		t.Fatalf("old client disabled retry: %v", err)
	}
	update = LLMUpdateFromSettings(next)
	if update.RetryRefusalLocally == nil || !*update.RetryRefusalLocally {
		t.Fatal("complete update omitted retry")
	}
	*update.RetryRefusalLocally = false
	next, err = applyLLMUpdate(next, update)
	if err != nil || next.RetryRefusalLocally {
		t.Fatalf("retry could not be disabled: %v", err)
	}
}

func TestPublicModelRoutingMatchesResolvedRoles(t *testing.T) {
	s := DefaultSettings()
	chat := ModelConnection{ID: "router", Name: "ChatGPT", Provider: LLMProviderOpenRouter, Model: "vendor/chat"}.Normalize()
	planner := ModelConnection{ID: "planner", Name: "Planner", Provider: LLMProviderChatGPT, Model: "account/model"}.Normalize()
	s.LLM.Connections = []ModelConnection{chat, planner}
	s.LLM.ConversationConnectionID = chat.ID
	s.LLM.MotionPlanner = MotionPlannerSettings{Provider: "connection", ConnectionID: planner.ID, ContextPolicy: ContextTechnical}
	routes := s.Public().ModelRouting
	if routes.LocalRetry != nil {
		t.Fatal("retry must be opt-in")
	}
	s.LLM.RetryRefusalLocally = true
	localRoute := s.Public().ModelRouting.LocalRetry
	if localRoute == nil || localRoute.Kind != "local" || localRoute.Model != s.LLM.Model {
		t.Fatalf("wrong retry destination: %+v", localRoute)
	}
	resolvedChat := s.LLM.ConversationSettings()
	resolvedPlanner, err := s.LLM.PlanningSettings()
	if err != nil || routes.Chat.Provider != resolvedChat.Provider || routes.Chat.Model != resolvedChat.Model || routes.Chat.EndpointHost != "openrouter.ai" || routes.Autopilot.Provider != resolvedPlanner.Provider || routes.Autopilot.Model != resolvedPlanner.Model || routes.Autopilot.ContextPolicy != ContextTechnical {
		t.Fatalf("routing projection disagrees with dispatch: %+v %v", routes, err)
	}
	s.LLM.MotionPlanner.Provider = MotionPlannerDecisions
	if route := s.Public().ModelRouting.Autopilot; route.Kind != "decisions" || route.ContextPolicy != ContextTechnical {
		t.Fatalf("Decisions context: %+v", route)
	}
}

func TestMissingModelConnectionNeverFallsBackToLocal(t *testing.T) {
	s := DefaultSettings()
	s.LLM.ConversationConnectionID = "missing"
	resolved := s.LLM.ConversationSettings()
	if !resolved.IsHosted() || resolved.Provider != "unavailable" {
		t.Fatalf("missing ID fell back: %+v", resolved)
	}
	if route := s.Public().ModelRouting.Chat; route.State != "unavailable" || route.Kind != "unavailable" || route.EndpointHost != "" {
		t.Fatalf("false routing claim: %+v", route)
	}
	s.LLM.MotionPlanner = MotionPlannerSettings{Provider: "connection", ConnectionID: "missing"}
	if route := s.Public().ModelRouting.Autopilot; route.State != "unavailable" {
		t.Fatalf("missing planner masked: %+v", route)
	}
	s.LLM.ConversationConnectionID = "local"
	s.LLM.Provider = LLMProviderOpenAI // corrupt/legacy data must not be labelled local
	if route := s.Public().ModelRouting.Chat; route.Kind == "local" || route.State != "unavailable" {
		t.Fatalf("hosted provider labelled local: %+v", route)
	}
}

func TestIncompleteActiveConnectionsCannotBeSaved(t *testing.T) {
	s := DefaultSettings().LLM
	s.Connections = []ModelConnection{{ID: "router", Name: "Router", Provider: LLMProviderOpenRouter}}
	s.Connections[0] = s.Connections[0].Normalize()
	if err := validateModelConnections(s); err != nil {
		t.Fatalf("unused configuration should be retainable: %v", err)
	}
	s.ConversationConnectionID = "router"
	if err := validateModelConnections(s); err == nil {
		t.Fatal("empty chat model admitted")
	}
	s.ConversationConnectionID = "local"
	s.MotionPlanner = MotionPlannerSettings{Provider: "connection", ConnectionID: "router"}
	if err := validateModelConnections(s); err == nil {
		t.Fatal("empty planner model admitted")
	}
}
