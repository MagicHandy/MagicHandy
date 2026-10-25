package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultAutopilotSettingsUseIndependentNaturalCadences(t *testing.T) {
	settings := DefaultAutopilotSettings()
	if settings.SpeechCadence != AutopilotSpeechNatural ||
		settings.MotionCadence != AutopilotMotionScaled ||
		settings.MotionChangeLevel != autopilotDefaultMotionChangeLevel ||
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

func TestAutopilotMotionChangeScaleWidensBothEnds(t *testing.T) {
	windows := [][2]int{
		{90, 240}, {60, 150}, {45, 120}, {20, 60}, {14, 45}, {10, 35}, {8, 24}, {8, 16},
	}
	settings := DefaultAutopilotSettings()
	for index, want := range windows {
		settings.MotionChangeLevel = index + 1
		minimum, maximum := settings.MotionWindow()
		if minimum != want[0] || maximum != want[1] {
			t.Fatalf("level %d window = %d..%d, want %d..%d",
				index+1, minimum, maximum, want[0], want[1])
		}
	}
}

func TestLegacyMotionCadenceMigratesToNumberedScale(t *testing.T) {
	for cadence, want := range map[string]int{
		AutopilotMotionSteady: 3, AutopilotMotionNatural: 4, AutopilotMotionDynamic: 6,
	} {
		stored := DefaultAutopilotSettings()
		stored.MotionCadence = cadence
		stored.MotionChangeLevel = 0
		resolved := applyMissingAutopilotDefaults(stored, DefaultAutopilotSettings())
		if resolved.MotionCadence != AutopilotMotionScaled || resolved.MotionChangeLevel != want {
			t.Fatalf("%s migrated to cadence=%s level=%d, want scaled/%d",
				cadence, resolved.MotionCadence, resolved.MotionChangeLevel, want)
		}
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
		{"motion level", func(s *AutopilotSettings) { s.MotionChangeLevel = 9 }, "motion change level"},
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

// Two switches, and the arc cannot exist without the tracking that produces it.
// Rejecting the combination keeps the settings document from expressing a state
// the runtime would have to silently ignore.
func TestSessionArcRequiresSessionTracking(t *testing.T) {
	settings := DefaultAutopilotSettings()
	settings.SessionArc = true
	settings.SessionTracking = false
	if err := validateAutopilotSettings(settings); err == nil {
		t.Fatal("arc without tracking should be rejected")
	}
	settings.SessionTracking = true
	if err := validateAutopilotSettings(settings); err != nil {
		t.Fatalf("arc with tracking should be accepted: %v", err)
	}
}

func TestSessionBuildupAcceptsAnyPositivePracticalDuration(t *testing.T) {
	for _, minutes := range []int{1, 5, 30, 180, 24 * 60} {
		settings := DefaultAutopilotSettings()
		settings.SessionArcMinutes = minutes
		if err := validateAutopilotSettings(settings); err != nil {
			t.Fatalf("%d minutes should be accepted: %v", minutes, err)
		}
	}
	for _, minutes := range []int{0, AutopilotMaximumArcMinutes + 1} {
		settings := DefaultAutopilotSettings()
		settings.SessionArcMinutes = minutes
		if err := validateAutopilotSettings(settings); err == nil {
			t.Fatalf("%d minutes should be rejected", minutes)
		}
	}
}

// A bool cannot tell "absent" from "explicitly false", so a document written
// before this field group existed would have run with tracking off while the
// documented default is on. The arc length doubles as the presence marker.
func TestPreArcDocumentAdoptsTheTrackingDefault(t *testing.T) {
	stored := AutopilotSettings{
		SpeechCadence:         AutopilotSpeechNatural,
		MotionCadence:         AutopilotMotionNatural,
		SpeechMotionAuthority: AutopilotSpeechMotionChatOnly,
		SpeechMinSeconds:      35,
		SpeechMaxSeconds:      120,
		MotionMinSeconds:      20,
		MotionMaxSeconds:      60,
	}
	resolved := applyMissingAutopilotDefaults(stored, DefaultAutopilotSettings())
	if !resolved.SessionTracking {
		t.Fatal("a pre-arc document should adopt the tracking default")
	}
	if resolved.SessionArcMinutes != AutopilotDefaultArcMinutes {
		t.Fatalf("arc minutes = %d, want the default", resolved.SessionArcMinutes)
	}
	if resolved.SessionArc {
		t.Fatal("the arc itself must stay opt-in")
	}
}

// Once the group has been saved once, an explicit false must survive.
func TestExplicitTrackingOffIsPreserved(t *testing.T) {
	stored := DefaultAutopilotSettings()
	stored.SessionTracking = false
	stored.SessionArc = false
	resolved := applyMissingAutopilotDefaults(stored, DefaultAutopilotSettings())
	if resolved.SessionTracking {
		t.Fatal("an explicit tracking-off choice was overwritten by the default")
	}
}

// Auto timing hands the horizon to a hosted planner, so it stays an explicit
// choice: off by default, kept when saved, and never dropped by backfill.
func TestAutoMotionTimingIsOptInAndSurvivesNormalization(t *testing.T) {
	if DefaultAutopilotSettings().AutoMotionTiming {
		t.Fatal("auto timing must default off")
	}
	stored := DefaultAutopilotSettings()
	stored.AutoMotionTiming = true
	if resolved := applyMissingAutopilotDefaults(stored, DefaultAutopilotSettings()); !resolved.AutoMotionTiming {
		t.Fatal("backfill dropped an explicit auto timing choice")
	}
	if err := validateAutopilotSettings(stored); err != nil {
		t.Fatalf("auto timing settings rejected: %v", err)
	}
	encoded, err := json.Marshal(stored)
	if err != nil || !strings.Contains(string(encoded), `"auto_motion_timing":true`) {
		t.Fatalf("encoded settings = %s, %v", encoded, err)
	}
	if AutopilotAutoTimingMinSeconds != AutopilotMinimumIntervalSeconds ||
		AutopilotAutoTimingMaxSeconds != AutopilotMaximumMotionSeconds {
		t.Fatalf("auto timing bounds = %d..%d", AutopilotAutoTimingMinSeconds, AutopilotAutoTimingMaxSeconds)
	}
}

// Preparing changes only when an external planner is asked, so it is on by
// default, including for documents saved before the field existed.
func TestPrepareNextDecisionDefaultsOnForOldDocuments(t *testing.T) {
	if !DefaultAutopilotSettings().PrepareNextDecision {
		t.Fatal("prepared decisions must default on")
	}
	legacy := []byte(`{"version":2,"autopilot":{"speech_cadence":"natural","motion_cadence":"scaled","motion_change_level":5,"speech_motion_authority":"chat_only","session_arc_minutes":30}}`)
	settings, _, err := loadSettingsFromBytes(legacy)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !settings.Autopilot.PrepareNextDecision || settings.Autopilot.MotionChangeLevel != 5 {
		t.Fatalf("legacy autopilot settings = %+v", settings.Autopilot)
	}
	explicit := []byte(`{"version":2,"autopilot":{"speech_cadence":"natural","motion_cadence":"scaled","motion_change_level":5,"speech_motion_authority":"chat_only","session_arc_minutes":30,"prepare_next_decision":false}}`)
	if settings, _, err := loadSettingsFromBytes(explicit); err != nil || settings.Autopilot.PrepareNextDecision {
		t.Fatalf("explicit off was not kept: %+v, %v", settings.Autopilot, err)
	}
}

func TestIntermediatePlanningDefaultsOnAndPreservesExplicitOff(t *testing.T) {
	if !DefaultAutopilotSettings().IntermediatePlanning {
		t.Fatal("existing Intermediate behavior must remain the default")
	}
	legacy, _, err := loadSettingsFromBytes([]byte(`{"version":2,"autopilot":{"speech_cadence":"natural","motion_change_level":5}}`))
	if err != nil || !legacy.Autopilot.IntermediatePlanning {
		t.Fatalf("old settings lost extra planning: %+v %v", legacy.Autopilot, err)
	}
	legacy.Autopilot.IntermediatePlanning = false
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	loaded, _, err := loadSettingsFromBytes(encoded)
	if err != nil || loaded.Autopilot.IntermediatePlanning || loaded.Autopilot.MotionChangeLevel != 5 {
		t.Fatalf("explicit off or other preferences changed on reload: %+v %v", loaded.Autopilot, err)
	}
}
