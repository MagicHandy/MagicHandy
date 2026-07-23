package semantic

import "testing"

func TestResolveStrokeProfileRiding(t *testing.T) {
	profile := ResolveStrokeProfile(LLMIntent{Action: ActionRiding, Location: LocationShaft, Intensity: 6})
	if profile.DownstrokeRatio >= profile.UpstrokeRatio {
		t.Fatalf("riding down=%.2f up=%.2f", profile.DownstrokeRatio, profile.UpstrokeRatio)
	}
}

func TestResolveStrokeProfileDeepthroat(t *testing.T) {
	profile := ResolveStrokeProfile(LLMIntent{Action: ActionDeepthroat, Location: LocationFull, Intensity: 8})
	if !profile.HasBottomBounce {
		t.Fatal("expected bottom bounce for deepthroat")
	}
}

func TestOrganicConfigFromIntentCarriesStrokeProfile(t *testing.T) {
	intent := LLMIntent{Action: ActionDeepthroat, Location: LocationTip, Intensity: 7}
	cfg, err := OrganicConfigFromIntent(intent, DefaultMotionPreferences(), 75)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.StrokeProfile.HasBottomBounce {
		t.Fatal("organic config should carry deepthroat bounce profile")
	}
}
