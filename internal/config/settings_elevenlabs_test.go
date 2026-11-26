package config

import (
	"encoding/json"
	"testing"
)

func TestRetiredElevenLabsVoiceMovesToTheCurrentDefault(t *testing.T) {
	settings := DefaultSettings()
	settings.Voice.ElevenLabsVoiceID = RetiredElevenLabsVoiceID
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Voice.ElevenLabsVoiceID != DefaultElevenLabsVoiceID {
		t.Fatalf("voice = %q, want the current default", normalized.Voice.ElevenLabsVoiceID)
	}

	settings.Voice.ElevenLabsVoiceID = "chosenVoice123"
	normalized, err = NormalizeSettings(settings)
	if err != nil || normalized.Voice.ElevenLabsVoiceID != "chosenVoice123" {
		t.Fatalf("a chosen voice must be kept: %q, %v", normalized.Voice.ElevenLabsVoiceID, err)
	}
}

func TestElevenLabsDeliveryValidation(t *testing.T) {
	value := func(v float64) *float64 { return &v }
	for name, tc := range map[string]struct {
		delivery ElevenLabsDelivery
		ok       bool
	}{
		"unset":            {ElevenLabsDelivery{}, true},
		"in range":         {ElevenLabsDelivery{Stability: value(0), Similarity: value(1), Style: value(0.4), Speed: value(0.7), Language: " EN "}, true},
		"stability high":   {ElevenLabsDelivery{Stability: value(1.01)}, false},
		"style negative":   {ElevenLabsDelivery{Style: value(-0.1)}, false},
		"speed too slow":   {ElevenLabsDelivery{Speed: value(0.5)}, false},
		"speed too fast":   {ElevenLabsDelivery{Speed: value(1.3)}, false},
		"language invalid": {ElevenLabsDelivery{Language: "english"}, false},
		"language digits":  {ElevenLabsDelivery{Language: "e1"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Voice.ElevenLabsDelivery = tc.delivery
			normalized, err := NormalizeSettings(settings)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if err == nil && tc.delivery.Language != "" && normalized.Voice.ElevenLabsDelivery.Language != "en" {
				t.Fatalf("language should be normalized, got %q", normalized.Voice.ElevenLabsDelivery.Language)
			}
		})
	}
}

func TestElevenLabsDeliveryRoundTripsThroughAFormPatch(t *testing.T) {
	settings, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	var patch SettingsPatch
	if err := json.Unmarshal([]byte(`{"changes":{"voice":{"elevenlabs_delivery":{"stability":0.35,"speed":0.9,"language":"en"}}},
		"expected":{"voice":{"elevenlabs_delivery":{}}}}`), &patch); err != nil {
		t.Fatal(err)
	}
	next, err := settings.ApplyPatch(patch)
	if err != nil {
		t.Fatal(err)
	}
	delivery := next.Voice.ElevenLabsDelivery
	if delivery.Stability == nil || *delivery.Stability != 0.35 || delivery.Speed == nil || *delivery.Speed != 0.9 ||
		delivery.Similarity != nil || delivery.Language != "en" {
		t.Fatalf("delivery = %+v", delivery)
	}
	public := next.Public().Voice.ElevenLabsDelivery
	if public.Stability == nil || *public.Stability != 0.35 {
		t.Fatalf("public delivery = %+v", public)
	}
	// The clone must not share pointers with the stored settings.
	*public.Stability = 0.9
	if *next.Voice.ElevenLabsDelivery.Stability != 0.35 {
		t.Fatal("public settings share delivery pointers with stored settings")
	}
}
