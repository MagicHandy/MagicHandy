package config

import "testing"

func TestLLMMotionGenerationModesValidateAndPersist(t *testing.T) {
	settings := DefaultSettings()
	if settings.LLM.MotionGenerationMode != LLMMotionModeLayered {
		t.Fatalf("default mode = %q, want layered", settings.LLM.MotionGenerationMode)
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

func TestFreshInstallStartsOnLayeredAndKeepsIt(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings, status := store.Snapshot()
	if !status.UsingDefaults || settings.LLM.MotionGenerationMode != LLMMotionModeLayered {
		t.Fatalf("fresh install mode = %q (defaults %t), want layered", settings.LLM.MotionGenerationMode, status.UsingDefaults)
	}
	// The first save records the mode, so a reload is not mistaken for a
	// document that predates the selector.
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore reload: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	if got, _ := reloaded.Snapshot(); got.LLM.MotionGenerationMode != LLMMotionModeLayered {
		t.Fatalf("reloaded fresh install mode = %q, want layered", got.LLM.MotionGenerationMode)
	}
}

func TestSavedLLMMotionModeSurvivesNewInstallDefaultChange(t *testing.T) {
	for _, mode := range []string{LLMMotionModeCreativeV2, LLMMotionModeLayered, LLMMotionModeDynamic, LLMMotionModePattern, LLMMotionModeOff} {
		t.Run(mode, func(t *testing.T) {
			settings, _, err := loadSettingsFromBytes([]byte(`{"llm":{"motion_generation_mode":"` + mode + `"}}`))
			if err != nil || settings.LLM.MotionGenerationMode != mode {
				t.Fatalf("saved mode %s changed to %s: %v", mode, settings.LLM.MotionGenerationMode, err)
			}
		})
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
