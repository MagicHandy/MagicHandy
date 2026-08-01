package config

import (
	"strings"
	"testing"
)

func TestVoiceUpdatePreservesAndReplacesTTSTone(t *testing.T) {
	current := DefaultSettings().Voice
	current.TTSTonePreset = TTSToneCustom
	current.TTSTonePrompt = "Speak with restrained anticipation."

	preserved := applyVoiceUpdate(current, VoiceUpdate{})
	if preserved.TTSTonePreset != TTSToneCustom || preserved.TTSTonePrompt != current.TTSTonePrompt {
		t.Fatalf("omitted tone update = %q %q", preserved.TTSTonePreset, preserved.TTSTonePrompt)
	}

	preset := TTSToneWarm
	prompt := "Retain this custom prompt for later."
	replaced := applyVoiceUpdate(current, VoiceUpdate{TTSTonePreset: &preset, TTSTonePrompt: &prompt})
	if replaced.TTSTonePreset != TTSToneWarm || replaced.TTSTonePrompt != prompt {
		t.Fatalf("explicit tone update = %q %q", replaced.TTSTonePreset, replaced.TTSTonePrompt)
	}
}

func TestTTSTonePresetsResolveToReviewedInstructions(t *testing.T) {
	want := map[string]string{
		TTSToneNatural:    "",
		TTSToneWarm:       "Speak in a warm, intimate tone with gentle pacing and natural emotional variation.",
		TTSTonePlayful:    "Speak in a playful, teasing tone with lively rhythm and expressive emphasis.",
		TTSToneTender:     "Speak softly in a tender, reassuring tone with calm pacing.",
		TTSToneCommanding: "Speak in a confident, commanding tone with deliberate pacing and clear emphasis.",
		TTSToneExcited:    "Speak with excited, energetic delivery, varied pitch, and brisk but intelligible pacing.",
		TTSToneCustom:     "Use close, breathy delivery.",
	}
	if got := TTSTonePresets(); len(got) != len(want) || got[0] != TTSToneNatural || got[len(got)-1] != TTSToneCustom {
		t.Fatalf("tone preset catalog = %v", got)
	}
	for preset, prompt := range want {
		voice := DefaultSettings().Voice
		voice.TTSTonePreset = preset
		voice.TTSTonePrompt = "  Use close, breathy delivery.  "
		if got := ResolveTTSTonePrompt(voice); got != prompt {
			t.Errorf("ResolveTTSTonePrompt(%q) = %q, want %q", preset, got, prompt)
		}
	}
}

func TestTTSToneValidation(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		prompt string
		want   string
	}{
		{name: "unknown preset", preset: "dramatic-ish", want: "unknown TTS tone preset"},
		{name: "empty custom", preset: TTSToneCustom, want: "custom TTS tone requires a prompt"},
		{name: "oversized custom", preset: TTSToneCustom, prompt: strings.Repeat("x", (2<<10)+1), want: "tone prompt must not exceed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Voice.TTSProvider = VoiceTTSProviderFasterQwen
			settings.Voice.TTSTonePreset = test.preset
			settings.Voice.TTSTonePrompt = test.prompt
			if _, err := NormalizeSettings(settings); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NormalizeSettings error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestEmptyCustomTTSToneDoesNotBlockOtherProviders(t *testing.T) {
	settings := DefaultSettings()
	settings.Voice.TTSProvider = VoiceTTSProviderOpenAICompat
	settings.Voice.TTSTonePreset = TTSToneCustom
	settings.Voice.TTSTonePrompt = ""
	if _, err := NormalizeSettings(settings); err != nil {
		t.Fatalf("inactive Qwen tone settings blocked compatible provider: %v", err)
	}
}

func TestCurrentSettingsWithoutTTSToneUseBackwardCompatibleDefault(t *testing.T) {
	settings, migrated, err := loadSettingsFromBytes([]byte(`{"version":2,"voice":{"tts_provider":"none","asr_provider":"none"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if migrated {
		t.Fatal("additive tone defaults should not require a schema migration")
	}
	if settings.Voice.TTSTonePreset != TTSToneNatural || ResolveTTSTonePrompt(settings.Voice) != "" {
		t.Fatalf("legacy tone default = %q %q", settings.Voice.TTSTonePreset, ResolveTTSTonePrompt(settings.Voice))
	}
}
