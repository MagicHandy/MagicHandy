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
		TTSToneTender:     "creak",
		TTSTonePlayful:    "extra beat",
		TTSToneCommanding: "settled authority",
		TTSToneExcited:    "lively energy",
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
		if !strings.Contains(got, ttsDeliveryFraming) && !strings.Contains(got, ttsAuthorityFraming) {
			t.Errorf("ResolveTTSTonePrompt(%q) = %q, missing a delivery framing clause", preset, got)
		}
		if other, duplicate := seen[got]; duplicate {
			t.Errorf("presets %q and %q resolve to the same instruction", preset, other)
		}
		seen[got] = preset
	}
}

// Three ways to ask a multilingual synthesizer for an accent by accident. All
// three surfaced from one report: Commanding arrived in an audibly foreign
// accent on seed 2783659410 and sounded timid on every other seed.
//
// Flattening the pitch contour is the prosody of a syllable-timed language, not
// of English, and it costs the tone its conviction besides -- a sentence that
// never resolves downward sounds tentative. Relaxing articulation gives up one
// of the strongest accent cues a synthesizer has. Shifting the pitch baseline
// changes the apparent speaker rather than the delivery. A preset rules out
// uptalk by asking for a falling close, and finds its energy in pitch movement
// and stress rather than in a raised voice or loosened diction.
func TestTTSTonePresetsAvoidAccentDriftLevers(t *testing.T) {
	banned := map[string]string{
		"level pitch":           "flattens the contour",
		"flat":                  "flattens the contour",
		"monotone":              "flattens the contour",
		"little pitch movement": "flattens the contour",
		"loose articulation":    "relaxes diction",
		"slurred":               "relaxes diction",
		"lazy":                  "relaxes diction",
		"every word":            "makes the word the prosodic unit",
		"each word separately":  "makes the word the prosodic unit",
		"rushed together":       "makes the word the prosodic unit",
		"lifted pitch":          "shifts the baseline",
		"raise the pitch":       "shifts the baseline",
		"higher-pitched":        "shifts the baseline",
	}
	voice := DefaultSettings().Voice
	for _, preset := range TTSTonePresets() {
		if preset == TTSToneCustom {
			continue // the user's own text is theirs to write
		}
		voice.TTSTonePreset = preset
		got := strings.ToLower(ResolveTTSTonePrompt(voice))
		for phrase, why := range banned {
			if strings.Contains(got, phrase) {
				t.Errorf("preset %q says %q, which %s and invites accent drift: %q", preset, phrase, why, got)
			}
		}
	}
}

// Quiet, slow, low, and falling all push the voice the same direction, and the
// bottom of that stack is where phonation gives out into press or creak, which is
// heard as straining. Tender asked for softly AND slowly AND low volume AND
// audible breath AND a falling close with nothing holding the voice up, and
// strained. Warm survives the same direction because it reduces on fewer axes and
// says "relaxed and unforced" outright.
//
// This cannot be a banned substring -- "quietly" is exactly right for Warm. What
// it checks is the pairing: any preset that asks the voice to back off has to
// also say something that keeps it supported.
func TestQuietTTSTonePresetsCarryAPhonationCue(t *testing.T) {
	// Reducers are volume and effort only, not pace. Commanding is unhurried at a
	// full chest-toned volume and is not backing off at all, so listing "slowly" or
	// "unhurried" here just produces a false positive on it.
	reducers := []string{"quiet", "soft", "gently", "low volume"}
	support := []string{"unforced", "relaxed", "supported", "never pressed", "open", "chest"}
	voice := DefaultSettings().Voice
	for _, preset := range TTSTonePresets() {
		if preset == TTSToneCustom {
			continue
		}
		voice.TTSTonePreset = preset
		got := strings.ToLower(ResolveTTSTonePrompt(voice))
		if !containsAny(got, reducers) || containsAny(got, support) {
			continue
		}
		t.Errorf("preset %q asks the voice to back off without any cue keeping it "+
			"supported, which is how Tender came out straining: %q", preset, got)
	}
}

func containsAny(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
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
