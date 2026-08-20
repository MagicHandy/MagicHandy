package config

import "testing"

func TestHandyModelProfilesNormalizeAndValidate(t *testing.T) {
	settings := DefaultSettings()
	settings.Motion.HandyModel = HandyModel2Pro
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		t.Fatalf("NormalizeSettings Handy 2 Pro: %v", err)
	}
	if normalized.Motion.HandyModel != HandyModel2Pro {
		t.Fatalf("Handy model = %q, want %q", normalized.Motion.HandyModel, HandyModel2Pro)
	}
	options := normalized.Public().Options.HandyModels
	want := []string{HandyModelOriginal, HandyModel2Standard, HandyModel2Pro}
	if len(options) != len(want) {
		t.Fatalf("Handy model options = %v, want %v", options, want)
	}
	for index := range want {
		if options[index] != want[index] {
			t.Fatalf("Handy model option %d = %q, want %q", index, options[index], want[index])
		}
	}

	settings.Motion.HandyModel = "handy_future_guess"
	if _, err := NormalizeSettings(settings); err == nil {
		t.Fatal("unknown Handy model was accepted")
	}
}
