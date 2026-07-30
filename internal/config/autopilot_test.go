package config

import (
	"strings"
	"testing"
)

func TestDefaultAutopilotSettingsUseIndependentNaturalCadences(t *testing.T) {
	settings := DefaultAutopilotSettings()
	if settings.SpeechCadence != AutopilotSpeechNatural ||
		settings.MotionCadence != AutopilotMotionNatural ||
		settings.SpeechMotionAuthority != AutopilotSpeechMotionChatOnly {
		t.Fatalf("defaults = %+v", settings)
	}
	if !settings.AdaptiveSpeechTiming || !settings.AdaptiveMotionTiming {
		t.Fatalf("adaptive timing defaults = %+v, want both enabled", settings)
	}
	speechMin, speechMax, enabled := settings.SpeechWindow()
	if !enabled || speechMin != 35 || speechMax != 120 {
		t.Fatalf("speech window = %d..%d enabled=%t", speechMin, speechMax, enabled)
	}
	motionMin, motionMax := settings.MotionWindow()
	if motionMin != 20 || motionMax != 60 {
		t.Fatalf("motion window = %d..%d", motionMin, motionMax)
	}
}

func TestAutopilotPresetAndCustomWindows(t *testing.T) {
	settings := DefaultAutopilotSettings()
	settings.SpeechCadence = AutopilotSpeechOff
	if minimum, maximum, enabled := settings.SpeechWindow(); enabled || minimum != 0 || maximum != 0 {
		t.Fatalf("off speech window = %d..%d enabled=%t", minimum, maximum, enabled)
	}
	settings.SpeechCadence = AutopilotSpeechCustom
	settings.SpeechMinSeconds = 21
	settings.SpeechMaxSeconds = 79
	if minimum, maximum, enabled := settings.SpeechWindow(); !enabled || minimum != 21 || maximum != 79 {
		t.Fatalf("custom speech window = %d..%d enabled=%t", minimum, maximum, enabled)
	}
	settings.MotionCadence = AutopilotMotionCustom
	settings.MotionMinSeconds = 13
	settings.MotionMaxSeconds = 44
	if minimum, maximum := settings.MotionWindow(); minimum != 13 || maximum != 44 {
		t.Fatalf("custom motion window = %d..%d", minimum, maximum)
	}
}

func TestNormalizeSettingsBackfillsAutopilotDocument(t *testing.T) {
	normalized, err := NormalizeSettings(Settings{})
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	if normalized.Autopilot != DefaultAutopilotSettings() {
		t.Fatalf("backfilled Autopilot = %+v", normalized.Autopilot)
	}
}

func TestValidateAutopilotSettingsRejectsUnsafeWindows(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AutopilotSettings)
		want   string
	}{
		{"short motion", func(s *AutopilotSettings) { s.MotionMinSeconds = 7 }, "between 8 and 300"},
		{"reversed speech", func(s *AutopilotSettings) { s.SpeechMinSeconds = 90; s.SpeechMaxSeconds = 30 }, "minimum cannot exceed"},
		{"unknown authority", func(s *AutopilotSettings) { s.SpeechMotionAuthority = "unbounded" }, "unknown Autopilot speech motion authority"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := DefaultSettings()
			test.mutate(&settings.Autopilot)
			_, err := NormalizeSettings(settings)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
