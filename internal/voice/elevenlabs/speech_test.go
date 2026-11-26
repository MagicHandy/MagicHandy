package elevenlabs

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitForSpeechPrefersNaturalBoundaries(t *testing.T) {
	if got := SplitForSpeech("  short reply  ", 100); len(got) != 1 || got[0] != "short reply" {
		t.Fatalf("short text = %q", got)
	}
	if got := SplitForSpeech("   ", 100); got != nil {
		t.Fatalf("blank text = %q", got)
	}

	text := "First sentence here. Second sentence follows it. Third sentence closes the thought."
	pieces := SplitForSpeech(text, 50)
	if len(pieces) < 2 {
		t.Fatalf("pieces = %q", pieces)
	}
	for _, piece := range pieces {
		if utf8.RuneCountInString(piece) > 50 {
			t.Fatalf("piece over the limit: %q", piece)
		}
		if !strings.HasSuffix(piece, ".") {
			t.Fatalf("piece should end at a sentence boundary: %q", piece)
		}
	}
	if strings.Join(pieces, " ") != text {
		t.Fatalf("splitting must not lose text: %q", pieces)
	}

	paragraphs := "Opening line that sets the scene.\r\n\r\nA new paragraph begins right here and continues."
	if got := SplitForSpeech(paragraphs, 60); len(got) != 2 || got[0] != "Opening line that sets the scene." {
		t.Fatalf("paragraph split = %q", got)
	}
}

func TestSplitForSpeechNeverBreaksAnAudioTag(t *testing.T) {
	text := "Words before the tag go here [whispers softly into your ear] and words after it continue on."
	// A tag longer than the whole limit cannot stay intact; real limits are
	// thousands of characters, so start above the tag's length.
	for limit := 40; limit < len(text); limit += 3 {
		for _, piece := range SplitForSpeech(text, limit) {
			if strings.Count(piece, "[") != strings.Count(piece, "]") {
				t.Fatalf("limit %d split a tag: %q", limit, piece)
			}
		}
	}
}

func TestSplitForSpeechHardCutsUnbrokenText(t *testing.T) {
	text := strings.Repeat("あ", 130)
	pieces := SplitForSpeech(text, 50)
	if len(pieces) != 3 || strings.Join(pieces, "") != text {
		t.Fatalf("pieces = %d", len(pieces))
	}
}

func TestProfilesFollowTheDocumentedModelBehavior(t *testing.T) {
	v4 := ProfileFor("eleven_v4", nil)
	if v4.Style || v4.Speed || v4.SpeakerBoost || !v4.Stability || !v4.Similarity || !v4.AudioTags || !v4.LanguageCode {
		t.Fatalf("eleven_v4 profile = %+v", v4)
	}
	if v2 := ProfileFor("eleven_multilingual_v2", nil); v2.LanguageCode || !v2.Style || !v2.Speed || v2.MaxTextRunes != 10000 {
		t.Fatalf("multilingual v2 profile = %+v", v2)
	}
	if v3 := ProfileFor("eleven_v3", nil); !v3.StabilityPreset || v3.MaxTextRunes != 5000 {
		t.Fatalf("eleven_v3 profile = %+v", v3)
	}
	flash := ProfileFor("eleven_flash_v2_5", &Model{ID: "eleven_flash_v2_5", CanUseSpeakerBoost: false, MaxTextLength: 2000})
	if flash.SpeakerBoost || flash.MaxTextRunes != 2000 || !flash.LanguageCode {
		t.Fatalf("live model data should narrow the profile: %+v", flash)
	}
	if unknown := ProfileFor("eleven_future_v9", nil); unknown.MaxTextRunes != defaultMaxTextRunes || unknown.Style {
		t.Fatalf("unknown model profile = %+v", unknown)
	}
}

func TestShapeDropsAndClampsSettings(t *testing.T) {
	high, low, speed := 1.7, -0.2, 2.0
	boost := false
	settings := &VoiceSettings{Stability: &high, SimilarityBoost: &low, Style: &high, Speed: &speed, UseSpeakerBoost: &boost}

	v2 := ProfileFor("eleven_multilingual_v2", nil).Shape(settings)
	if *v2.Stability != 1 || *v2.SimilarityBoost != 0 || *v2.Style != 1 || *v2.Speed != 1.2 || *v2.UseSpeakerBoost {
		t.Fatalf("multilingual v2 shaped = %+v", v2)
	}
	v4 := ProfileFor("eleven_v4", nil).Shape(settings)
	if v4.Style != nil || v4.Speed != nil || v4.UseSpeakerBoost != nil || v4.Stability == nil {
		t.Fatalf("eleven_v4 shaped = %+v", v4)
	}
	middling := 0.4
	if v3 := ProfileFor("eleven_v3", nil).Shape(&VoiceSettings{Stability: &middling}); *v3.Stability != 0.4 {
		t.Fatalf("the API accepts any eleven_v3 stability in range; got %v", *v3.Stability)
	}
	if shaped := ProfileFor("eleven_v4", nil).Shape(&VoiceSettings{Speed: &speed}); shaped != nil {
		t.Fatalf("settings a model ignores entirely should be omitted, got %+v", shaped)
	}
	if ProfileFor("eleven_v4", nil).Shape(nil) != nil {
		t.Fatal("nil settings stay nil")
	}
}

func TestStreamSpeechValidatesBeforeSending(t *testing.T) {
	client, err := New(Options{APIKey: "k", BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, voice := range []string{"", "../x", "a b", strings.Repeat("v", 200)} {
		if _, err := client.StreamSpeech(t.Context(), voice, SpeechRequest{Text: "hi"}); !IsKind(err, KindInvalid) {
			t.Fatalf("voice %q: err = %v", voice, err)
		}
	}
	if _, err := client.StreamSpeech(t.Context(), "voice", SpeechRequest{Text: "   "}); !IsKind(err, KindInvalid) {
		t.Fatalf("blank text: err = %v", err)
	}
}
