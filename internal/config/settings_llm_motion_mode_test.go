package config

import "testing"

func TestLLMMotionGenerationModesValidateAndPersist(t *testing.T) {
	settings := DefaultSettings()
	if settings.LLM.MotionGenerationMode != LLMMotionModePattern {
		t.Fatalf("conservative default mode = %q, want pattern", settings.LLM.MotionGenerationMode)
	}
	mode := LLMMotionModeDynamic
	update := LLMUpdateFromSettings(settings.LLM)
	update.MotionGenerationMode = &mode
	next, err := applyLLMUpdate(settings.LLM, update)
	if err != nil || next.MotionGenerationMode != LLMMotionModeDynamic {
		t.Fatalf("dynamic update = %+v, %v", next, err)
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
