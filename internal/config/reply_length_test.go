package config

import "testing"

func TestReplyLengthDefaultsPreservesAndValidates(t *testing.T) {
	// Payloads that predate the field resolve to balanced, which composes no
	// prompt change.
	settings := DefaultSettings()
	settings.LLM.ReplyLength = ""
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	if normalized.LLM.ReplyLength != LLMReplyLengthBalanced {
		t.Fatalf("legacy reply length = %q, want balanced", normalized.LLM.ReplyLength)
	}

	saved := normalized
	saved.LLM.ReplyLength = LLMReplyLengthShort
	update := SettingsUpdate{
		Server:      saved.Server,
		Device:      DeviceUpdate{HSPDispatchOwner: saved.Device.HSPDispatchOwner, FirmwareAPIRequirement: saved.Device.FirmwareAPIRequirement, APIApplicationIDSource: saved.Device.APIApplicationIDSource},
		Motion:      saved.Motion,
		LLM:         LLMUpdateFromSettings(saved.LLM),
		Diagnostics: saved.Diagnostics,
	}
	update.LLM.ReplyLength = nil
	next, err := saved.ApplyUpdate(update)
	if err != nil || next.LLM.ReplyLength != LLMReplyLengthShort {
		t.Fatalf("an omitted reply length clobbered the saved one: %q, %v", next.LLM.ReplyLength, err)
	}
	detailed := " Detailed "
	update.LLM.ReplyLength = &detailed
	if next, err = saved.ApplyUpdate(update); err != nil || next.LLM.ReplyLength != LLMReplyLengthDetailed {
		t.Fatalf("reply length replace = %q, %v", next.LLM.ReplyLength, err)
	}
	invalid := "novel"
	update.LLM.ReplyLength = &invalid
	if _, err := saved.ApplyUpdate(update); err == nil {
		t.Fatal("an unknown reply length was accepted")
	}
}

func TestDetailedRepliesRaiseOnlyASmallOutputBudget(t *testing.T) {
	settings := DefaultSettings().LLM
	settings.MaxOutputTokens = 256
	if got := settings.ChatMaxOutputTokens(LLMReplyLengthDetailed); got != DetailedReplyMinOutputTokens {
		t.Fatalf("detailed budget = %d, want %d", got, DetailedReplyMinOutputTokens)
	}
	for _, length := range []string{LLMReplyLengthShort, LLMReplyLengthBalanced} {
		if got := settings.ChatMaxOutputTokens(length); got != 256 {
			t.Fatalf("%s budget = %d, want the saved 256", length, got)
		}
	}
	settings.MaxOutputTokens = 1024
	if got := settings.ChatMaxOutputTokens(LLMReplyLengthDetailed); got != 1024 {
		t.Fatalf("detailed lowered a larger saved budget to %d", got)
	}
}

// A connection's capability floor applies to its own requests: DeepSeek's
// Advanced replies ran past 256 tokens about 40% of the time. The saved local
// limit and Simple connections keep the saved budget.
func TestConnectionOutputFloorFollowsItsCapability(t *testing.T) {
	settings := DefaultSettings().LLM
	settings.MaxOutputTokens = 256
	if got := settings.ChatMaxOutputTokens(LLMReplyLengthBalanced); got != 256 {
		t.Fatalf("local budget = %d, want the saved 256", got)
	}
	for level, want := range map[string]int{
		ModelCapabilitySimple: 256, ModelCapabilityIntermediate: 512,
		ModelCapabilityAdvancedNoReasoning: 1024, ModelCapabilityAdvanced: 1024, "": 1024,
	} {
		connection := ModelConnection{ID: "router", Name: "Router", Provider: LLMProviderOpenRouter, Model: "provider/model", CapabilityLevel: level}.Normalize()
		settings.ActiveConnection = &connection
		if got := settings.ChatMaxOutputTokens(LLMReplyLengthBalanced); got != want {
			t.Errorf("%q connection budget = %d, want %d", level, got, want)
		}
	}
	settings.MaxOutputTokens = 2048
	if got := settings.ChatMaxOutputTokens(LLMReplyLengthShort); got != 2048 {
		t.Fatalf("the floor lowered a larger saved budget to %d", got)
	}
}
