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
	if settings.SessionArc {
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

// The buildup no longer needs a tracking switch: session facts always reach
// the planner, so the arc stands on its own.
func TestSessionArcStandsOnItsOwn(t *testing.T) {
	settings := DefaultAutopilotSettings()
	settings.SessionArc = true
	if err := validateAutopilotSettings(settings); err != nil {
		t.Fatalf("the buildup alone was rejected: %v", err)
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

// A document written before the buildup existed adopts its length, and the
// buildup itself stays the person's choice.
func TestPreArcDocumentAdoptsTheBuildupDefaults(t *testing.T) {
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
	if resolved.SessionArcMinutes != AutopilotDefaultArcMinutes {
		t.Fatalf("arc minutes = %d, want the default", resolved.SessionArcMinutes)
	}
	if resolved.SessionArc {
		t.Fatal("the arc itself must stay opt-in")
	}
}

// The retired timing switches load from older documents without effect: the
// model's timing answer, prepared decisions and session facts are always on.
func TestRetiredTimingSwitchesAreIgnored(t *testing.T) {
	stored := []byte(`{"version":2,"autopilot":{"speech_cadence":"natural","motion_cadence":"scaled","motion_change_level":5,"speech_motion_authority":"chat_only","session_arc_minutes":30,"adaptive_motion_timing":false,"adaptive_speech_timing":false,"auto_motion_timing":false,"prepare_next_decision":false,"session_tracking":false}}`)
	settings, _, err := loadSettingsFromBytes(stored)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if settings.Autopilot.MotionChangeLevel != 5 {
		t.Fatalf("autopilot settings = %+v", settings.Autopilot)
	}
	encoded, _ := json.Marshal(settings.Autopilot)
	for _, retired := range []string{"adaptive_motion_timing", "adaptive_speech_timing", "auto_motion_timing", "prepare_next_decision", "session_tracking"} {
		if strings.Contains(string(encoded), retired) {
			t.Fatalf("%s is still saved: %s", retired, encoded)
		}
	}
}

// A planner that chooses its own timing may not ask again sooner than half the
// shortest ordinary stretch at the person's change rate.
func TestAutoTimingFloorFollowsTheChangeRate(t *testing.T) {
	settings := DefaultAutopilotSettings()
	for level, want := range map[int]int{1: 45, 4: 10, 8: AutopilotAutoTimingMinSeconds} {
		settings.MotionChangeLevel = level
		minimum, _ := settings.MotionWindow()
		if got := settings.AutoTimingFloorSeconds(); got != max(want, minimum/2, AutopilotAutoTimingMinSeconds) || got < AutopilotAutoTimingMinSeconds {
			t.Fatalf("level %d floor = %d (window minimum %d)", level, got, minimum)
		}
	}
	if AutopilotAutoTimingMaxSeconds != AutopilotMaximumMotionSeconds {
		t.Fatalf("auto timing ceiling = %d", AutopilotAutoTimingMaxSeconds)
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
