package semantic

import (
	"encoding/json"
	"testing"
)

func TestDefaultMotionPreferences(t *testing.T) {
	prefs := DefaultMotionPreferences()
	tip := prefs.Zones[ZoneTip]
	if tip.Min != 0.7 || tip.Max != 1.0 {
		t.Fatalf("tip zone = %+v, want 0.7..1.0", tip)
	}
	if prefs.ActionOverrides[ActionDeepthroat] != ZoneFull {
		t.Fatalf("deepthroat override = %q, want full", prefs.ActionOverrides[ActionDeepthroat])
	}
}

func TestLoadMotionPreferencesRoundTrip(t *testing.T) {
	custom := DefaultMotionPreferences()
	custom.Zones[ZoneTip] = ZoneRange{Min: 0.75, Max: 0.95}
	raw, err := json.Marshal(custom)
	if err != nil {
		t.Fatal(err)
	}
	loaded := LoadMotionPreferences(raw)
	got := loaded.Zones[ZoneTip]
	if got.Min != 0.75 || got.Max != 0.95 {
		t.Fatalf("loaded tip = %+v, want 0.75..0.95", got)
	}
}

func TestResolveMotionBoundsMatrix(t *testing.T) {
	custom := DefaultMotionPreferences()
	custom.Zones[ZoneTip] = ZoneRange{Min: 0.75, Max: 0.95}

	cases := []struct {
		name    string
		intent  LLMIntent
		prefs   MotionPreferences
		wantMin float64
		wantMax float64
		wantErr bool
	}{
		{
			name:    "deepthroat_tip_uses_full",
			intent:  LLMIntent{Action: ActionDeepthroat, Location: LocationTip, Intensity: 8},
			wantMin: 0.0,
			wantMax: 1.0,
		},
		{
			name:    "handjob_tip_uses_tip",
			intent:  LLMIntent{Action: ActionHandjob, Location: LocationTip, Intensity: 5},
			wantMin: 0.7,
			wantMax: 1.0,
		},
		{
			name:    "riding_shaft",
			intent:  LLMIntent{Action: ActionRiding, Location: LocationShaft, Intensity: 6},
			wantMin: 0.3,
			wantMax: 0.7,
		},
		{
			name:    "oral_base_override_full",
			intent:  LLMIntent{Action: ActionOral, Location: LocationBase, Intensity: 4},
			wantMin: 0.0,
			wantMax: 1.0,
		},
		{
			name:    "titjob_full",
			intent:  LLMIntent{Action: ActionTitjob, Location: LocationFull, Intensity: 3},
			wantMin: 0.0,
			wantMax: 1.0,
		},
		{
			name:    "custom_tip_range",
			intent:  LLMIntent{Action: ActionHandjob, Location: LocationTip, Intensity: 7},
			prefs:   custom,
			wantMin: 0.75,
			wantMax: 0.95,
		},
		{
			name:    "invalid_action",
			intent:  LLMIntent{Action: "kiss", Location: LocationTip, Intensity: 5},
			wantErr: true,
		},
		{
			name:    "invalid_location",
			intent:  LLMIntent{Action: ActionHandjob, Location: "head", Intensity: 5},
			wantErr: true,
		},
		{
			name:    "intensity_low",
			intent:  LLMIntent{Action: ActionHandjob, Location: LocationTip, Intensity: 0},
			wantErr: true,
		},
		{
			name:    "intensity_high",
			intent:  LLMIntent{Action: ActionHandjob, Location: LocationTip, Intensity: 11},
			wantErr: true,
		},
		{
			name:    "location_base_default",
			intent:  LLMIntent{Action: ActionHandjob, Location: LocationBase, Intensity: 5},
			wantMin: 0.0,
			wantMax: 0.3,
		},
		{
			name:    "riding_tip_not_overridden",
			intent:  LLMIntent{Action: ActionRiding, Location: LocationTip, Intensity: 9},
			wantMin: 0.7,
			wantMax: 1.0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefs := tc.prefs
			if prefs.Zones == nil {
				prefs = DefaultMotionPreferences()
			}
			min, max, err := ResolveMotionBounds(tc.intent, prefs)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if min != tc.wantMin || max != tc.wantMax {
				t.Fatalf("bounds = %.2f..%.2f, want %.2f..%.2f", min, max, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestOrganicConfigFromIntentMatchesResolver(t *testing.T) {
	intent := LLMIntent{Action: ActionHandjob, Location: LocationTip, Intensity: 6}
	min, max, err := ResolveMotionBounds(intent, DefaultMotionPreferences())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := OrganicConfigFromIntent(intent, DefaultMotionPreferences(), 70)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StrokeMin != min*100 || cfg.StrokeMax != max*100 {
		t.Fatalf("organic stroke = %.1f..%.1f, want %.1f..%.1f", cfg.StrokeMin, cfg.StrokeMax, min*100, max*100)
	}
}

func TestRegiaoLocationRoundTrip(t *testing.T) {
	cases := map[string]LocationName{
		"meio_cabeca": LocationShaft,
		"cabeca":      LocationTip,
		"base":        LocationBase,
		"full":        LocationFull,
	}
	for regiao, want := range cases {
		if got := RegiaoToLocation(regiao); got != want {
			t.Fatalf("RegiaoToLocation(%q) = %q, want %q", regiao, got, want)
		}
	}
	if LocationToRegiao(LocationTip) != "cabeca" {
		t.Fatalf("LocationToRegiao(tip) = %q", LocationToRegiao(LocationTip))
	}
}

func TestValidateLLMIntentEachEnum(t *testing.T) {
	actions := []ActionName{ActionOral, ActionHandjob, ActionRiding, ActionTitjob, ActionDeepthroat}
	locations := []LocationName{LocationBase, LocationShaft, LocationTip, LocationFull}
	for _, action := range actions {
		for _, location := range locations {
			intent := LLMIntent{Action: action, Location: location, Intensity: 5}
			if err := ValidateLLMIntent(intent); err != nil {
				t.Fatalf("%s/%s: %v", action, location, err)
			}
		}
	}
}
