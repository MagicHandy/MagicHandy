package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLocalCapabilityUpdatesPreserveLegacyAndEnforceBudgets(t *testing.T) {
	original := DefaultSettings().LLM
	for _, level := range ModelCapabilityLevels() {
		t.Run(level, func(t *testing.T) {
			update := LLMUpdateFromSettings(original)
			update.LocalCapabilityLevel = &level
			ctx, out := 16384, 256
			update.LlamaCPPContextSize, update.MaxOutputTokens = &ctx, &out
			next, err := applyLLMUpdate(original, update)
			if err != nil || validateLLMSettings(next) != nil {
				t.Fatalf("update failed: %+v %v", next, err)
			}
			if next.ModelCapabilityLevel() != level || next.LlamaCPPContextSize != max(ctx, MinimumLocalContext(level)) || next.MaxOutputTokens != max(out, MinimumLocalOutput(level)) {
				t.Fatalf("wrong allocation: %+v", next)
			}
			data, _ := json.Marshal(next)
			var roundTrip LLMUpdate
			if err := json.Unmarshal(data, &roundTrip); err != nil {
				t.Fatal(err)
			}
			roundTrip.LocalCapabilityLevel = nil // an older client must not downgrade it
			kept, err := applyLLMUpdate(next, roundTrip)
			if err != nil || !reflect.DeepEqual(kept, next) {
				t.Fatalf("omission changed tier: %v", err)
			}
			bad := next
			bad.LocalCapabilityLevel = "automatic"
			if validateLLMSettings(bad) == nil {
				t.Fatal("unknown capability accepted")
			}
			if level != ModelCapabilitySimple {
				bad = next
				bad.LlamaCPPContextSize = 16384
				if validateLLMSettings(bad) == nil {
					t.Fatal("undersized context accepted")
				}
				bad = next
				bad.MaxOutputTokens = 256
				if validateLLMSettings(bad) == nil {
					t.Fatal("undersized output accepted")
				}
			}
		})
	}
	if original.LocalCapabilityLevel != "" || original.AdvancedMotion() || original.IntermediateMotion() {
		t.Fatal("legacy settings silently upgraded")
	}
}

func TestCapabilityFollowsResolvedModelRoleNotProviderName(t *testing.T) {
	s := DefaultSettings().LLM
	s.Provider, s.Model = LLMProviderOllama, "user-chosen"
	s.LocalCapabilityLevel, s.LlamaCPPContextSize, s.MaxOutputTokens = ModelCapabilityAdvanced, 65536, 1024
	s.MotionGenerationMode = LLMMotionModeLayered
	routes := s.ModelRouting()
	if routes.Chat.Kind != "local" || routes.Chat.Provider != LLMProviderOllama || !routes.Chat.Features.AutoTiming || !routes.Chat.Features.IndependentVibration || !routes.Chat.Features.ResponseRepair || !routes.Autopilot.Features.PrepareDecisions {
		t.Fatalf("advanced local features missing or route changed: %+v", routes)
	}
	c := ModelConnection{ID: "external", Provider: LLMProviderOpenRouter, Model: "remote/model"}.Normalize()
	s.Connections, s.ConversationConnectionID = []ModelConnection{c}, c.ID
	s.LocalCapabilityLevel = ModelCapabilitySimple
	s.MotionPlanner = MotionPlannerSettings{Provider: MotionPlannerLocal}
	s.RetryRefusalLocally = true
	routes = s.ModelRouting()
	if routes.Chat.Kind != "hosted" || !routes.Chat.Features.AutoTiming || routes.Autopilot.Kind != "local" || routes.Autopilot.Features.AutoTiming || routes.LocalRetry == nil || routes.LocalRetry.Features.Level != ModelCapabilitySimple {
		t.Fatalf("role capabilities leaked: %+v", routes)
	}
	s.ConversationConnectionID = "local"
	s.LocalCapabilityLevel = ModelCapabilityIntermediate
	if features := s.ModelRouting().Chat.Features; features.Level != ModelCapabilityIntermediate || features.AutoTiming || features.ResponseRepair || features.IndependentVibration {
		t.Fatalf("intermediate acquired advanced tools: %+v", features)
	}
	s.LocalCapabilityLevel, s.MotionGenerationMode = ModelCapabilityAdvanced, LLMMotionModeOff
	if s.ModelRouting().Chat.Features.IndependentVibration {
		t.Fatal("motion-off advertised vibration")
	}
}
