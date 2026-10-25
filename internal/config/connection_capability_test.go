package config

import "testing"

func lmStudio(level string, context int) ModelConnection {
	return ModelConnection{ID: "lm-studio", Name: "LM Studio", Provider: LLMProviderCompatible, BaseURL: "http://127.0.0.1:1234/v1", Model: "qwen3-14b", NoAuthentication: true, CapabilityLevel: level, ContextWindow: context}.Normalize()
}

func TestConnectionCapabilityFollowsTheModelNotTheTransport(t *testing.T) {
	for _, tc := range []struct {
		level, want  string
		minimum      int
		advancedOnly bool
	}{
		{"", ModelCapabilityAdvanced, MinimumExternalContext, true},
		{"simple", ModelCapabilitySimple, 16384, false},
		{" Intermediate ", ModelCapabilityIntermediate, 32768, false},
		{"advanced", ModelCapabilityAdvanced, MinimumExternalContext, true},
	} {
		connection := lmStudio(tc.level, 0)
		if err := connection.Validate(); err != nil {
			t.Fatal(tc.level, err)
		}
		s := DefaultSettings().LLM
		s.MotionGenerationMode = LLMMotionModeLayered
		s.Connections = []ModelConnection{connection}
		s.ConversationConnectionID = connection.ID
		chat := s.ConversationSettings()
		if !chat.IsHosted() || chat.ModelCapabilityLevel() != tc.want {
			t.Fatalf("%q resolved to %q", tc.level, chat.ModelCapabilityLevel())
		}
		features := chat.ModelFeatures()
		if features.Level != tc.want || features.MinimumContext != tc.minimum {
			t.Fatalf("%q features = %+v", tc.level, features)
		}
		if features.AutoTiming != tc.advancedOnly || features.PrepareDecisions != tc.advancedOnly || features.ResponseRepair != tc.advancedOnly || features.IndependentVibration != tc.advancedOnly {
			t.Fatalf("%q features = %+v", tc.level, features)
		}
		if route := s.ModelRouting().Chat; route.Kind != "hosted" || route.Features.Level != tc.want {
			t.Fatalf("%q route = %+v", tc.level, route)
		}
		if s.ModelCapabilityLevel() != ModelCapabilitySimple {
			t.Fatal("a connection's level changed the local model's level")
		}
	}
}

func TestConnectionLevelSetsItsRequestBudget(t *testing.T) {
	for _, tc := range []struct {
		level                     string
		saved, want               int
		messages, historyBytesCap int
	}{
		{"simple", 0, 16384, 64, 24000},
		{"simple", 16384, 16384, 64, 24000},
		{"simple", 32768, 32768, 64, 24000},
		{"intermediate", 16384, 32768, 64, 24000},
		{"intermediate", 131072, 131072, 256, 96000},
		{"advanced", 32768, MinimumExternalContext, 64, 24000},
		{"", 16384, MinimumExternalContext, 64, 24000},
	} {
		connection := lmStudio(tc.level, tc.saved)
		if err := connection.Validate(); err != nil {
			t.Fatal(tc, err)
		}
		budget, messages, historyBytes := connection.ConversationWindow()
		if budget != tc.want || messages != tc.messages || historyBytes != tc.historyBytesCap {
			t.Errorf("%+v: budget=%d messages=%d bytes=%d", tc, budget, messages, historyBytes)
		}
	}
}

func TestConnectionCapabilityRejectsUnknownLevelsAndBudgets(t *testing.T) {
	if err := lmStudio("expert", 0).Validate(); err == nil {
		t.Fatal("unknown capability admitted")
	}
	for _, size := range []int{8192, 20000, 64000} {
		if err := lmStudio("simple", size).Validate(); err == nil {
			t.Fatal("unlisted budget admitted", size)
		}
	}
	unnormalized := ModelConnection{ID: "lm-studio", Name: "LM Studio", Provider: LLMProviderCompatible, BaseURL: "http://127.0.0.1:1234/v1", NoAuthentication: true, CapabilityLevel: ModelCapabilityAdvanced, ContextWindow: 16384, OutputMode: "auto", DataCollection: "deny"}
	if err := unnormalized.Validate(); err == nil {
		t.Fatal("an Advanced connection admitted a 16K budget")
	}
}
