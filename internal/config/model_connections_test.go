package config

import (
	"reflect"
	"testing"
)

func TestHostedRolesPreserveLocalDefaultsAndLegacyWrites(t *testing.T) {
	settings := DefaultSettings().LLM
	settings.Model = "saved-local-model"
	connection := ModelConnection{ID: "router", Name: "Router", Provider: LLMProviderOpenRouter, Model: "provider/model"}.Normalize()
	settings.Connections, settings.ConversationConnectionID = []ModelConnection{connection}, connection.ID
	settings.MotionPlanner = MotionPlannerSettings{Provider: "connection", ConnectionID: "local", ContextPolicy: ContextConversation}
	conversation := settings.ConversationSettings()
	if !conversation.IsHosted() || conversation.Model != connection.Model || settings.Provider != LLMProviderLlamaCPP || settings.Model != "saved-local-model" {
		t.Fatal("hosted role overwrote local defaults")
	}
	localMotion, err := settings.PlanningSettings()
	if err != nil || localMotion.IsHosted() || localMotion.Model != "saved-local-model" {
		t.Fatalf("explicit local motion = %+v, %v", localMotion, err)
	}
	settings.MotionPlanner.Provider = MotionPlannerLocal
	localMotion, err = settings.PlanningSettings()
	if err != nil || localMotion.IsHosted() {
		t.Fatal("legacy local role resolved to cloud conversation")
	}
	update := LLMUpdateFromSettings(settings)
	update.Connections, update.ConversationConnectionID, update.MotionPlanner = nil, nil, nil
	next, err := applyLLMUpdate(settings, update)
	if err != nil || !reflect.DeepEqual(next.Connections, settings.Connections) || next.ConversationConnectionID != connection.ID || next.MotionPlanner != settings.MotionPlanner {
		t.Fatalf("legacy write erased hosted settings: %+v, %v", next, err)
	}
	comparison := conversation.WithModel("comparison-model")
	if comparison.ActiveConnection.Model != "comparison-model" || conversation.ActiveConnection.Model != connection.Model {
		t.Fatal("temporary comparison changed the saved connection")
	}
}

func TestModelConnectionValidationRejectsCredentialDestinations(t *testing.T) {
	for _, endpoint := range []string{"http://remote.example/v1", "https://user:secret@example.test/v1", "https://example.test/v1?key=private", "https://example.test/v1#fragment"} {
		connection := ModelConnection{ID: "compatible", Name: "Endpoint", Provider: LLMProviderCompatible, BaseURL: endpoint, Model: "model"}.Normalize()
		if err := connection.Validate(); err == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	for _, endpoint := range []string{"http://127.0.0.1:8080/v1", "http://[::1]:8080/v1", "https://example.test/v1"} {
		connection := ModelConnection{ID: "compatible", Name: "Endpoint", Provider: LLMProviderCompatible, BaseURL: endpoint, Model: "model"}.Normalize()
		if err := connection.Validate(); err != nil {
			t.Errorf("valid endpoint rejected: %s, %v", endpoint, err)
		}
	}
}

func TestModelConnectionReasoningStaysWithSupportedProtocol(t *testing.T) {
	connection := ModelConnection{ID: "openai", Name: "OpenAI", Provider: LLMProviderOpenAI, Model: "account-model", ReasoningEffort: "low"}.Normalize()
	if err := connection.Validate(); err != nil {
		t.Fatal(err)
	}
	connection.ReasoningEffort = "invented"
	if err := connection.Validate(); err == nil {
		t.Fatal("unknown effort accepted")
	}
	connection.ReasoningEffort, connection.Provider = "low", LLMProviderOpenRouter
	if err := connection.Validate(); err == nil {
		t.Fatal("unsupported protocol silently ignored effort")
	}
}
