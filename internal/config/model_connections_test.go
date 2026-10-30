package config

import (
	"encoding/json"
	"reflect"
	"strings"
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
	if err := connection.Normalize().Validate(); err != nil {
		t.Fatal(err)
	}
	connection.Provider = LLMProviderCompatible
	if err := connection.Validate(); err == nil {
		t.Fatal("unsupported protocol silently ignored effort")
	}
}

func TestOpenRouterReasoningEnabledPersistsFalseAndRejectsConflict(t *testing.T) {
	off := false
	c := ModelConnection{ID: "router", Name: "Router", Provider: LLMProviderOpenRouter, Model: "fixture", ReasoningEnabled: &off}.Normalize()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var restored ModelConnection
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ReasoningEnabled == nil || *restored.ReasoningEnabled {
		t.Fatal("explicit Off became provider default")
	}
	c.ReasoningEffort = "low"
	if c.Validate() == nil {
		t.Fatal("conflicting reasoning settings accepted")
	}
	c.ReasoningEffort = ""
	c.Provider = LLMProviderOpenAI
	if c.Validate() == nil {
		t.Fatal("enabled silently ignored by another protocol")
	}
}

func TestOpenRouterRoutingOrderIsValidatedAndPersisted(t *testing.T) {
	connection := ModelConnection{ID: "router", Name: "Router", Provider: LLMProviderOpenRouter, Model: "provider/model", ProviderSort: " Throughput "}.Normalize()
	if connection.ProviderSort != "throughput" || connection.Validate() != nil {
		t.Fatalf("routing order = %q, %v", connection.ProviderSort, connection.Validate())
	}
	encoded, _ := json.Marshal(connection)
	var restored ModelConnection
	if json.Unmarshal(encoded, &restored) != nil || restored.ProviderSort != "throughput" {
		t.Fatalf("routing order did not persist: %s", encoded)
	}
	connection.ProviderSort = ""
	if encoded, _ = json.Marshal(connection); connection.Validate() != nil || strings.Contains(string(encoded), "provider_sort") {
		t.Fatalf("default routing must stay valid and unsaved: %s", encoded)
	}
	for _, invalid := range []ModelConnection{
		{ID: "router", Name: "Router", Provider: LLMProviderOpenRouter, Model: "provider/model", ProviderSort: "fastest"},
		{ID: "chatgpt", Name: "ChatGPT", Provider: LLMProviderChatGPT, Model: "gpt", ProviderSort: "throughput"},
		{ID: "compatible", Name: "Compatible", Provider: LLMProviderCompatible, BaseURL: "http://127.0.0.1:8000/v1", Model: "model", ProviderSort: "latency"},
	} {
		if invalid.Normalize().Validate() == nil {
			t.Errorf("accepted routing order %q for %s", invalid.ProviderSort, invalid.Provider)
		}
	}
}
