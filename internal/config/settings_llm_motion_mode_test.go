package config

import "testing"

func TestLLMMotionGenerationModesValidateAndPersist(t *testing.T) {
	settings := DefaultSettings()
	if settings.LLM.MotionGenerationMode != LLMMotionModeDynamic {
		t.Fatalf("default mode = %q, want dynamic", settings.LLM.MotionGenerationMode)
	}
	mode := LLMMotionModePattern
	update := LLMUpdateFromSettings(settings.LLM)
	update.MotionGenerationMode = &mode
	next, err := applyLLMUpdate(settings.LLM, update)
	if err != nil || next.MotionGenerationMode != LLMMotionModePattern {
		t.Fatalf("pattern update = %+v, %v", next, err)
	}
	settings.LLM = next
	if _, err := NormalizeSettings(settings); err != nil {
		t.Fatalf("normalize dynamic mode: %v", err)
	}
	settings.LLM.MotionGenerationMode = "invented"
	if _, err := NormalizeSettings(settings); err == nil {
		t.Fatal("unknown LLM motion generation mode was accepted")
	}
}

func TestMissingLLMMotionModeAdoptsCreativeUnlessMotionWasDisabled(t *testing.T) {
	settings, _, err := loadSettingsFromBytes([]byte(`{}`))
	if err != nil {
		t.Fatalf("load legacy defaults: %v", err)
	}
	if settings.LLM.MotionGenerationMode != LLMMotionModeDynamic {
		t.Fatalf("legacy missing mode = %q, want dynamic", settings.LLM.MotionGenerationMode)
	}

	settings, _, err = loadSettingsFromBytes([]byte(`{
		"llm":{"motion_capabilities":{"motion":false,"patterns":false,"area_focus":false,"experimental_patterns":false}}
	}`))
	if err != nil {
		t.Fatalf("load legacy chat-only settings: %v", err)
	}
	if settings.LLM.MotionGenerationMode != LLMMotionModeOff {
		t.Fatalf("legacy chat-only mode = %q, want off", settings.LLM.MotionGenerationMode)
	}
}
