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
		TTSToneWarm:       "quietly",
		TTSToneTender:     "gently",
		TTSTonePlayful:    "smile in the voice",
		TTSToneCommanding: "quiet finality",
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
		if !strings.Contains(got, ttsDeliveryFraming) {
			t.Errorf("ResolveTTSTonePrompt(%q) = %q, missing the delivery framing clause", preset, got)
		}
		// Pinned separately from the constant: merging two framing clauses once
		// dropped these three words, and Warm came back sounding like a sports
		// announcer. Rewording the rest of the framing is fine; losing this is not.
		if !strings.Contains(got, "not a performance or an announcement") {
			t.Errorf("ResolveTTSTonePrompt(%q) lost the announcer negation: %q", preset, got)
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

// Every built-in preset carries the ease anchor, and stays short.
//
// Both halves come from the same finding. Whatever a preset asks for has to hold
// across a whole reply, and the reported failures in real use -- Commanding and
// Tender straining, Warm turning shouty, Excited going nasal -- were the voice
// being driven past what it can sustain. The anchor names that ceiling. The
// length cap is why the ceiling is reachable: every clause is one more constraint
// to satisfy simultaneously, and a preset stacking five or six leaves only an
// extreme corner of the model's range to satisfy them all in.
//
// The earlier presets tested clean and failed in use because the preview button
// speaks four words, over which there is barely one intonation contour to get
// wrong. sampleTTSPreviewLength guards the preview text for the same reason.
func TestTTSTonePresetsStayShortAndAnchored(t *testing.T) {
	// Generous: the longest preset body at the time of writing is around 180
	// characters, so this catches a return to five- and six-clause instructions
	// without failing on ordinary rewording.
	const maxBodyLength = 240
	voice := DefaultSettings().Voice
	for _, preset := range TTSTonePresets() {
		if preset == TTSToneCustom || preset == TTSToneNatural {
			continue // Custom is the user's to write; Natural resolves empty.
		}
		voice.TTSTonePreset = preset
		got := ResolveTTSTonePrompt(voice)
		if !strings.Contains(got, ttsEaseAnchor) {
			t.Errorf("preset %q is missing the ease anchor, which is what keeps it "+
				"sustainable across a whole reply: %q", preset, got)
		}
		body := strings.TrimSpace(strings.NewReplacer(ttsEaseAnchor, "", ttsDeliveryFraming, "").Replace(got))
		if len(body) > maxBodyLength {
			t.Errorf("preset %q body is %d characters, over the %d cap; every extra "+
				"clause is another constraint the voice has to hold for a whole reply: %q",
				preset, len(body), maxBodyLength, body)
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
