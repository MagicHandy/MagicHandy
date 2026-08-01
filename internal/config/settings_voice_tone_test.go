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

// The preset wording is tuning copy, so this pins the behavior around it rather
// than the sentences: Natural stays silent for backward compatibility, Custom
// passes the user's text through trimmed, and every built-in preset resolves to
// its own distinct instruction carrying the shared unperformed-delivery clause.
// A keyword per preset still catches a swapped or duplicated mapping.
func TestTTSTonePresetsResolveToReviewedInstructions(t *testing.T) {
	keyword := map[string]string{
		TTSToneWarm:       "unhurried",
		TTSTonePlayful:    "quicker",
		TTSToneTender:     "slowly",
		TTSToneCommanding: "level pitch",
		TTSToneExcited:    "briskly",
	}
	presets := TTSTonePresets()
	if len(presets) != len(keyword)+2 || presets[0] != TTSToneNatural || presets[len(presets)-1] != TTSToneCustom {
		t.Fatalf("tone preset catalog = %v", presets)
	}

	voice := DefaultSettings().Voice
	voice.TTSTonePrompt = "  Use close, breathy delivery.  "
	voice.TTSTonePreset = TTSToneNatural
	if got := ResolveTTSTonePrompt(voice); got != "" {
		t.Errorf("Natural must stay silent, got %q", got)
	}
	voice.TTSTonePreset = TTSToneCustom
	if got := ResolveTTSTonePrompt(voice); got != "Use close, breathy delivery." {
		t.Errorf("Custom = %q, want the trimmed user prompt", got)
	}

	seen := map[string]string{}
	for preset, want := range keyword {
		voice.TTSTonePreset = preset
		got := ResolveTTSTonePrompt(voice)
		if !strings.Contains(got, want) {
			t.Errorf("ResolveTTSTonePrompt(%q) = %q, want it to mention %q", preset, got, want)
		}
		if !strings.Contains(got, ttsDeliveryFraming) {
			t.Errorf("ResolveTTSTonePrompt(%q) = %q, missing the shared delivery framing", preset, got)
		}
		if other, duplicate := seen[got]; duplicate {
			t.Errorf("presets %q and %q resolve to the same instruction", preset, other)
		}
		seen[got] = preset
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
